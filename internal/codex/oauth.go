package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultChatGPTBaseURL = "https://chatgpt.com/backend-api"
	chatGPTUsagePath      = "/wham/usage"
	codexUsagePath        = "/api/codex/usage"
	refreshEndpoint       = "https://auth.openai.com/oauth/token"
	codexOAuthClientID    = "app_EMoamEEZ73f0CkXaXp7hrann"
)

func fetchOAuth(ctx context.Context, opts Options) (*Snapshot, error) {
	now := time.Now()
	creds, err := loadCredentials(opts)
	if err != nil {
		return nil, err
	}

	if creds.needsRefresh(now) && !opts.NoRefresh {
		refreshed, err := refreshCredentials(ctx, opts, creds)
		if err != nil {
			return nil, err
		}
		creds = refreshed
		if err := saveCredentials(creds, now); err != nil {
			return nil, fmt.Errorf("save refreshed Codex credentials: %w", err)
		}
	}

	usageURL, err := resolveUsageURL(opts)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "codexstat")
	if creds.AccountID != "" {
		req.Header.Set("ChatGPT-Account-Id", creds.AccountID)
	}

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch Codex OAuth usage: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("Codex OAuth token rejected with HTTP %d; run `codex` to re-authenticate", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("Codex usage API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload usageResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode Codex usage response: %w", err)
	}

	snapshot := snapshotFromUsageResponse(payload, creds, now)
	return finalizeSnapshot(snapshot, SourceOAuth, now)
}

func refreshCredentials(ctx context.Context, opts Options, creds credentials) (credentials, error) {
	if creds.RefreshToken == "" {
		return creds, nil
	}
	body, err := json.Marshal(map[string]string{
		"client_id":     codexOAuthClientID,
		"grant_type":    "refresh_token",
		"refresh_token": creds.RefreshToken,
		"scope":         "openid profile email",
	})
	if err != nil {
		return credentials{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, refreshEndpoint, bytes.NewReader(body))
	if err != nil {
		return credentials{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return credentials{}, fmt.Errorf("refresh Codex OAuth token: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return credentials{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return credentials{}, refreshFailure(resp.StatusCode, data)
	}

	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return credentials{}, fmt.Errorf("decode token refresh response: %w", err)
	}
	if parsed.AccessToken != "" {
		creds.AccessToken = parsed.AccessToken
	}
	if parsed.RefreshToken != "" {
		creds.RefreshToken = parsed.RefreshToken
	}
	if parsed.IDToken != "" {
		creds.IDToken = parsed.IDToken
	}
	now := time.Now()
	creds.LastRefresh = &now
	return creds, nil
}

func refreshFailure(status int, data []byte) error {
	var payload map[string]any
	_ = json.Unmarshal(data, &payload)
	code := ""
	if errObj, _ := payload["error"].(map[string]any); errObj != nil {
		code = stringField(errObj, "code")
	}
	if code == "" {
		code = stringField(payload, "error", "code")
	}
	switch strings.ToLower(code) {
	case "refresh_token_expired":
		return errors.New("Codex refresh token expired; run `codex` to log in again")
	case "refresh_token_reused":
		return errors.New("Codex refresh token was already used; run `codex` to log in again")
	case "invalid_grant", "refresh_token_invalidated":
		return errors.New("Codex refresh token was revoked; run `codex` to log in again")
	}
	return fmt.Errorf("Codex token refresh returned HTTP %d", status)
}

func resolveUsageURL(opts Options) (string, error) {
	base := defaultChatGPTBaseURL
	if path, err := configFilePath(opts); err == nil {
		if data, readErr := os.ReadFile(path); readErr == nil {
			if parsed := parseChatGPTBaseURL(string(data)); parsed != "" {
				base = parsed
			}
		}
	}

	normalized := normalizeChatGPTBaseURL(base)
	path := codexUsagePath
	if strings.Contains(normalized, "/backend-api") {
		path = chatGPTUsagePath
	}
	full := normalized + path
	if _, err := url.ParseRequestURI(full); err != nil {
		return "", fmt.Errorf("invalid Codex usage URL %q: %w", full, err)
	}
	return full, nil
}

func parseChatGPTBaseURL(contents string) string {
	for _, raw := range strings.Split(contents, "\n") {
		line := strings.TrimSpace(stripComment(raw))
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "chatgpt_base_url" {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		return strings.TrimSpace(value)
	}
	return ""
}

func stripComment(line string) string {
	inSingle := false
	inDouble := false
	escaped := false
	for i, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if r == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if r == '#' && !inSingle && !inDouble {
			return line[:i]
		}
	}
	return line
}

func normalizeChatGPTBaseURL(value string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(value), "/")
	if trimmed == "" {
		trimmed = defaultChatGPTBaseURL
	}
	if (strings.HasPrefix(trimmed, "https://chatgpt.com") || strings.HasPrefix(trimmed, "https://chat.openai.com")) &&
		!strings.Contains(trimmed, "/backend-api") {
		trimmed += "/backend-api"
	}
	return trimmed
}

type usageResponse struct {
	PlanType             string                `json:"plan_type"`
	RateLimit            *rateLimitDetails     `json:"rate_limit"`
	Credits              *creditDetails        `json:"credits"`
	AdditionalRateLimits []additionalRateLimit `json:"additional_rate_limits"`
}

type rateLimitDetails struct {
	PrimaryWindow   *windowSnapshot `json:"primary_window"`
	SecondaryWindow *windowSnapshot `json:"secondary_window"`
}

type windowSnapshot struct {
	UsedPercent        flexibleFloat `json:"used_percent"`
	ResetAt            int64         `json:"reset_at"`
	LimitWindowSeconds int           `json:"limit_window_seconds"`
}

type additionalRateLimit struct {
	LimitName      string            `json:"limit_name"`
	MeteredFeature string            `json:"metered_feature"`
	RateLimit      *rateLimitDetails `json:"rate_limit"`
}

type creditDetails struct {
	HasCredits bool           `json:"has_credits"`
	Unlimited  bool           `json:"unlimited"`
	Balance    *flexibleFloat `json:"balance"`
}

type flexibleFloat float64

func (f *flexibleFloat) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" {
		return nil
	}
	if len(text) >= 2 && text[0] == '"' && text[len(text)-1] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		text = strings.TrimSpace(value)
	}
	var value float64
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return err
	}
	*f = flexibleFloat(value)
	return nil
}

func (f flexibleFloat) value() float64 {
	return float64(f)
}

func snapshotFromUsageResponse(payload usageResponse, creds credentials, now time.Time) *Snapshot {
	primary, secondary := normalizeWindows(
		windowFromUsageSnapshot(payload.RateLimit, true, now),
		windowFromUsageSnapshot(payload.RateLimit, false, now),
	)

	var credits *Credits
	if payload.Credits != nil {
		credits = &Credits{
			HasCredits: payload.Credits.HasCredits,
			Unlimited:  payload.Credits.Unlimited,
		}
		if payload.Credits.Balance != nil {
			credits.Balance = ptr(payload.Credits.Balance.value())
		}
	}

	return &Snapshot{
		UpdatedAt: now,
		Account:   accountFromCredentials(creds, payload.PlanType),
		Session:   primary,
		Weekly:    secondary,
		Extra:     extraWindowsFromAdditionalLimits(payload.AdditionalRateLimits, now),
		Credits:   credits,
	}
}

func windowFromUsageSnapshot(details *rateLimitDetails, primary bool, now time.Time) *Window {
	if details == nil {
		return nil
	}
	if primary {
		return windowFromUsageWindow(details.PrimaryWindow, now)
	} else {
		return windowFromUsageWindow(details.SecondaryWindow, now)
	}
}

func windowFromUsageWindow(snapshot *windowSnapshot, now time.Time) *Window {
	if snapshot == nil {
		return nil
	}
	window := usageWindow(snapshot, now)
	return &window
}

func usageWindow(snapshot *windowSnapshot, now time.Time) Window {
	windowMinutes := optionalInt(snapshot.LimitWindowSeconds / 60)
	var resetsAt *time.Time
	if snapshot.ResetAt > 0 {
		reset := time.Unix(snapshot.ResetAt, 0)
		resetsAt = &reset
	}
	return makeWindow(snapshot.UsedPercent.value(), windowMinutes, resetsAt, now)
}

func optionalInt(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

func extraWindowsFromAdditionalLimits(entries []additionalRateLimit, now time.Time) []NamedWindow {
	if len(entries) == 0 {
		return nil
	}
	usedIDs := make(map[string]bool)
	var out []NamedWindow
	for _, entry := range entries {
		out = append(out, namedWindowsFromAdditionalLimit(entry, usedIDs, now)...)
	}
	return out
}

func namedWindowsFromAdditionalLimit(entry additionalRateLimit, usedIDs map[string]bool, now time.Time) []NamedWindow {
	if entry.RateLimit == nil {
		return nil
	}

	if isSparkLimit(entry) {
		var out []NamedWindow
		if window, ok := namedWindow("codex-spark", "Codex Spark 5-hour", entry.RateLimit.PrimaryWindow, usedIDs, now); ok {
			out = append(out, window)
		}
		if window, ok := namedWindow("codex-spark-weekly", "Codex Spark Weekly", entry.RateLimit.SecondaryWindow, usedIDs, now); ok {
			out = append(out, window)
		}
		return out
	}

	snapshot := entry.RateLimit.PrimaryWindow
	if snapshot == nil {
		snapshot = entry.RateLimit.SecondaryWindow
	}
	if snapshot == nil {
		return nil
	}
	idSource := firstNonEmpty(entry.MeteredFeature, entry.LimitName)
	if idSource == "" {
		return nil
	}
	title := firstNonEmpty(entry.LimitName, entry.MeteredFeature, "Codex extra limit")
	if window, ok := namedWindow("codex-"+slug(idSource), title, snapshot, usedIDs, now); ok {
		return []NamedWindow{window}
	}
	return nil
}

func namedWindow(id string, title string, snapshot *windowSnapshot, usedIDs map[string]bool, now time.Time) (NamedWindow, bool) {
	if id == "" || usedIDs[id] || snapshot == nil {
		return NamedWindow{}, false
	}
	usedIDs[id] = true
	return NamedWindow{
		ID:     id,
		Title:  title,
		Window: usageWindow(snapshot, now),
	}, true
}

func isSparkLimit(entry additionalRateLimit) bool {
	return strings.Contains(strings.ToLower(entry.LimitName+" "+entry.MeteredFeature), "spark")
}

func slug(value string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

package codex

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	errAuthNotFound  = errors.New("Codex auth.json not found; run `codex` to log in")
	errMissingTokens = errors.New("Codex auth.json exists but contains no OAuth tokens")
)

type credentials struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	AccountID    string
	LastRefresh  *time.Time
	AuthPath     string
	raw          map[string]any
}

func (c credentials) needsRefresh(now time.Time) bool {
	if c.RefreshToken == "" {
		return false
	}
	if c.LastRefresh == nil {
		return true
	}
	return now.Sub(*c.LastRefresh) > 8*24*time.Hour
}

func loadCredentials(opts Options) (credentials, error) {
	path, err := authFilePath(opts)
	if err != nil {
		return credentials{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return credentials{}, errAuthNotFound
	}
	if err != nil {
		return credentials{}, err
	}
	creds, err := parseCredentials(data)
	if err != nil {
		return credentials{}, err
	}
	creds.AuthPath = path
	return creds, nil
}

func parseCredentials(data []byte) (credentials, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return credentials{}, fmt.Errorf("decode Codex auth.json: %w", err)
	}

	if apiKey, _ := raw["OPENAI_API_KEY"].(string); strings.TrimSpace(apiKey) != "" {
		return credentials{
			AccessToken: strings.TrimSpace(apiKey),
			raw:         raw,
		}, nil
	}

	tokens, ok := raw["tokens"].(map[string]any)
	if !ok {
		return credentials{}, errMissingTokens
	}

	accessToken := stringField(tokens, "access_token", "accessToken")
	refreshToken := stringField(tokens, "refresh_token", "refreshToken")
	if accessToken == "" {
		return credentials{}, errMissingTokens
	}

	lastRefresh := parseTimeString(stringField(raw, "last_refresh", "lastRefresh"))
	return credentials{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		IDToken:      stringField(tokens, "id_token", "idToken"),
		AccountID:    stringField(tokens, "account_id", "accountId"),
		LastRefresh:  lastRefresh,
		raw:          raw,
	}, nil
}

func saveCredentials(creds credentials, now time.Time) error {
	if creds.AuthPath == "" {
		return errors.New("missing auth path")
	}
	raw := creds.raw
	if raw == nil {
		raw = make(map[string]any)
	}

	tokens := map[string]any{
		"access_token":  creds.AccessToken,
		"refresh_token": creds.RefreshToken,
	}
	if creds.IDToken != "" {
		tokens["id_token"] = creds.IDToken
	}
	if creds.AccountID != "" {
		tokens["account_id"] = creds.AccountID
	}
	raw["tokens"] = tokens
	raw["last_refresh"] = now.UTC().Format(time.RFC3339Nano)

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	mode := os.FileMode(0o600)
	if info, err := os.Stat(creds.AuthPath); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(creds.AuthPath), 0o700); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(creds.AuthPath), ".auth.json.")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, creds.AuthPath)
}

func authFilePath(opts Options) (string, error) {
	root, err := codexHome(opts)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "auth.json"), nil
}

func configFilePath(opts Options) (string, error) {
	root, err := codexHome(opts)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "config.toml"), nil
}

func codexHome(opts Options) (string, error) {
	if strings.TrimSpace(opts.CodexHome) != "" {
		return expandHome(strings.TrimSpace(opts.CodexHome), opts.Env), nil
	}
	if value := strings.TrimSpace(opts.Env["CODEX_HOME"]); value != "" {
		return expandHome(value, opts.Env), nil
	}
	home := strings.TrimSpace(opts.Env["HOME"])
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(home, ".codex"), nil
}

func expandHome(path string, env map[string]string) string {
	if path == "~" {
		if home := env["HOME"]; home != "" {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home := env["HOME"]; home != "" {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func stringField(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func parseTimeString(value string) *time.Time {
	if value == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return &parsed
		}
	}
	return nil
}

func accountFromCredentials(creds credentials, planFallback string) *Account {
	account := &Account{
		AccountID: creds.AccountID,
		Plan:      planFallback,
	}

	if payload := parseJWT(creds.IDToken); payload != nil {
		if profile, _ := payload["https://api.openai.com/profile"].(map[string]any); profile != nil {
			account.Email = firstNonEmpty(account.Email, stringField(profile, "email"))
		}
		if auth, _ := payload["https://api.openai.com/auth"].(map[string]any); auth != nil {
			account.Plan = firstNonEmpty(account.Plan, stringField(auth, "chatgpt_plan_type"))
			account.AccountID = firstNonEmpty(account.AccountID, stringField(auth, "chatgpt_account_id"))
		}
		account.Email = firstNonEmpty(account.Email, stringField(payload, "email"))
		account.Plan = firstNonEmpty(account.Plan, stringField(payload, "chatgpt_plan_type"))
		account.AccountID = firstNonEmpty(account.AccountID, stringField(payload, "chatgpt_account_id"))
	}

	if account.Email == "" && account.Plan == "" && account.AccountID == "" {
		return nil
	}
	return account
}

func parseJWT(token string) map[string]any {
	if token == "" {
		return nil
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil
		}
	}
	var out map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil
	}
	return out
}

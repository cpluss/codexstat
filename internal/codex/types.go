package codex

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type Source string

const (
	SourceAuto  Source = "auto"
	SourceOAuth Source = "oauth"
	SourceCLI   Source = "cli"
)

func ParseSource(value string) (Source, error) {
	switch Source(strings.ToLower(strings.TrimSpace(value))) {
	case "", SourceAuto:
		return SourceAuto, nil
	case SourceOAuth:
		return SourceOAuth, nil
	case SourceCLI:
		return SourceCLI, nil
	default:
		return "", fmt.Errorf("unknown source %q; expected auto, oauth, or cli", value)
	}
}

type Options struct {
	Source     Source
	CodexHome  string
	CodexBin   string
	Timeout    time.Duration
	NoRefresh  bool
	Env        map[string]string
	HTTPClient *http.Client
}

type Snapshot struct {
	Provider  string        `json:"provider"`
	Source    Source        `json:"source"`
	UpdatedAt time.Time     `json:"updated_at"`
	Account   *Account      `json:"account,omitempty"`
	Session   *Window       `json:"session,omitempty"`
	Weekly    *Window       `json:"weekly,omitempty"`
	Extra     []NamedWindow `json:"extra_rate_windows,omitempty"`
	Credits   *Credits      `json:"credits,omitempty"`
	Warnings  []string      `json:"warnings,omitempty"`
}

type Account struct {
	Email     string `json:"email,omitempty"`
	Plan      string `json:"plan,omitempty"`
	AccountID string `json:"account_id,omitempty"`
}

type Window struct {
	UsedPercent      float64    `json:"used_percent"`
	RemainingPercent float64    `json:"remaining_percent"`
	WindowMinutes    *int       `json:"window_minutes,omitempty"`
	ResetsAt         *time.Time `json:"resets_at,omitempty"`
	ResetIn          string     `json:"reset_in,omitempty"`
}

type NamedWindow struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Window Window `json:"window"`
}

type Credits struct {
	HasCredits bool     `json:"has_credits"`
	Unlimited  bool     `json:"unlimited"`
	Balance    *float64 `json:"balance,omitempty"`
}

func normalizedOptions(opts Options) Options {
	if opts.Source == "" {
		opts.Source = SourceAuto
	}
	if opts.CodexBin == "" {
		opts.CodexBin = "codex"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 15 * time.Second
	}
	if opts.Env == nil {
		opts.Env = environMap()
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: opts.Timeout}
	}
	return opts
}

func environMap() map[string]string {
	env := make(map[string]string)
	for _, pair := range os.Environ() {
		key, value, ok := strings.Cut(pair, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}

func envSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func ptr[T any](value T) *T {
	return &value
}

func makeWindow(used float64, windowMinutes *int, resetsAt *time.Time, now time.Time) Window {
	used = clamp(used, 0, 100)
	window := Window{
		UsedPercent:      used,
		RemainingPercent: clamp(100-used, 0, 100),
		WindowMinutes:    windowMinutes,
		ResetsAt:         resetsAt,
	}
	if resetsAt != nil {
		window.ResetIn = resetCountdown(*resetsAt, now)
	}
	return window
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

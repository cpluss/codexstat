package codex

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func Fetch(ctx context.Context, opts Options) (*Snapshot, error) {
	opts = normalizedOptions(opts)

	switch opts.Source {
	case SourceOAuth:
		return fetchOAuth(ctx, opts)
	case SourceCLI:
		return fetchCLI(ctx, opts)
	case SourceAuto:
		return fetchAuto(ctx, opts)
	default:
		return nil, fmt.Errorf("unsupported source %q", opts.Source)
	}
}

func fetchAuto(ctx context.Context, opts Options) (*Snapshot, error) {
	oauthSnapshot, oauthErr := fetchOAuth(ctx, opts)
	if oauthErr == nil {
		return oauthSnapshot, nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) {
		return nil, oauthErr
	}

	cliSnapshot, cliErr := fetchCLI(ctx, opts)
	if cliErr == nil {
		cliSnapshot.Warnings = append(cliSnapshot.Warnings, "oauth unavailable: "+shortError(oauthErr))
		return cliSnapshot, nil
	}

	return nil, fmt.Errorf("oauth failed: %w; cli failed: %v", oauthErr, cliErr)
}

func finalizeSnapshot(snapshot *Snapshot, source Source, now time.Time) (*Snapshot, error) {
	snapshot.Provider = "codex"
	snapshot.Source = source
	if snapshot.UpdatedAt.IsZero() {
		snapshot.UpdatedAt = now
	}
	if snapshot.Session == nil && snapshot.Weekly == nil && len(snapshot.Extra) == 0 && snapshot.Credits == nil {
		return nil, errNoCodexStats
	}
	return snapshot, nil
}

func shortError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

var errNoCodexStats = errors.New("no Codex stats found")

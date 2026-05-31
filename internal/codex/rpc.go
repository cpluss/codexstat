package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

func fetchCLI(ctx context.Context, opts Options) (*Snapshot, error) {
	now := time.Now()
	client, err := newRPCClient(ctx, opts)
	if err != nil {
		return nil, err
	}
	defer client.close()

	if err := client.initialize(ctx); err != nil {
		return nil, err
	}
	limits, err := client.fetchRateLimits(ctx)
	if err != nil {
		recovered, recoverErr := recoverSnapshotFromRPCError(err, now)
		if recoverErr != nil {
			return nil, recoverErr
		}
		return recovered, nil
	}
	account, _ := client.fetchAccount(ctx)

	primary, secondary := normalizeWindows(windowFromRPC(limits.RateLimits.Primary, now), windowFromRPC(limits.RateLimits.Secondary, now))
	snapshot := &Snapshot{
		UpdatedAt: now,
		Session:   primary,
		Weekly:    secondary,
		Account:   accountFromRPC(account, limits.RateLimits.PlanType),
		Credits:   creditsFromRPC(limits.RateLimits.Credits),
	}
	return finalizeSnapshot(snapshot, SourceCLI, now)
}

type rpcClient struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	lines   chan []byte
	stderr  bytes.Buffer
	nextID  int
	waitErr chan error
	once    sync.Once
}

func newRPCClient(ctx context.Context, opts Options) (*rpcClient, error) {
	bin := opts.CodexBin
	if !strings.Contains(bin, "/") {
		resolved, err := exec.LookPath(bin)
		if err != nil {
			return nil, fmt.Errorf("Codex CLI not found on PATH; install `codex` or pass --codex-bin: %w", err)
		}
		bin = resolved
	}

	cmd := exec.CommandContext(ctx, bin, "-s", "read-only", "-a", "untrusted", "app-server")
	cmd.Env = envSlice(opts.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	client := &rpcClient{
		cmd:     cmd,
		stdin:   stdin,
		lines:   make(chan []byte, 128),
		nextID:  1,
		waitErr: make(chan error, 1),
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start Codex app-server: %w", err)
	}

	go client.scanLines(stdout)
	go func() {
		_, _ = io.Copy(&client.stderr, stderr)
	}()
	go func() {
		client.waitErr <- cmd.Wait()
		close(client.waitErr)
	}()

	return client, nil
}

func (c *rpcClient) scanLines(stdout io.Reader) {
	defer close(c.lines)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		c.lines <- line
	}
}

func (c *rpcClient) close() {
	c.once.Do(func() {
		_ = c.stdin.Close()
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		<-c.waitErr
	})
}

func (c *rpcClient) initialize(ctx context.Context) error {
	_, err := c.request(ctx, "initialize", map[string]any{
		"clientInfo": map[string]string{
			"name":    "codexstat",
			"version": "0.1.0",
		},
	}, 8*time.Second)
	if err != nil {
		return err
	}
	return c.notify("initialized", map[string]any{})
}

func (c *rpcClient) fetchRateLimits(ctx context.Context) (rpcRateLimitsResponse, error) {
	var out rpcRateLimitsResponse
	result, err := c.request(ctx, "account/rateLimits/read", nil, 4*time.Second)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(result, &out)
	return out, err
}

func (c *rpcClient) fetchAccount(ctx context.Context) (rpcAccountResponse, error) {
	var out rpcAccountResponse
	result, err := c.request(ctx, "account/read", nil, 4*time.Second)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(result, &out)
	return out, err
}

func (c *rpcClient) request(ctx context.Context, method string, params any, timeout time.Duration) (json.RawMessage, error) {
	id := c.nextID
	c.nextID++

	if err := c.send(map[string]any{
		"id":     id,
		"method": method,
		"params": paramsOrEmpty(params),
	}); err != nil {
		return nil, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-reqCtx.Done():
			return nil, fmt.Errorf("Codex RPC timed out waiting for %s", method)
		case err := <-c.waitErr:
			if err == nil {
				err = errors.New("codex app-server exited")
			}
			if stderr := strings.TrimSpace(c.stderr.String()); stderr != "" {
				return nil, fmt.Errorf("%w: %s", err, stderr)
			}
			return nil, err
		case line, ok := <-c.lines:
			if !ok {
				return nil, errors.New("codex app-server closed stdout")
			}
			var message rpcMessage
			if err := json.Unmarshal(line, &message); err != nil {
				continue
			}
			if message.ID == nil {
				continue
			}
			var messageID int
			if err := json.Unmarshal(message.ID, &messageID); err != nil || messageID != id {
				continue
			}
			if message.Error != nil {
				return nil, errors.New(message.Error.Message)
			}
			if len(message.Result) == 0 {
				return nil, errors.New("Codex RPC response missing result")
			}
			return message.Result, nil
		}
	}
}

func (c *rpcClient) notify(method string, params any) error {
	return c.send(map[string]any{
		"method": method,
		"params": paramsOrEmpty(params),
	})
}

func (c *rpcClient) send(payload map[string]any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = c.stdin.Write(data)
	return err
}

func paramsOrEmpty(params any) any {
	if params == nil {
		return map[string]any{}
	}
	return params
}

type rpcMessage struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Message string `json:"message"`
}

type rpcRateLimitsResponse struct {
	RateLimits rpcRateLimitSnapshot `json:"rateLimits"`
}

type rpcRateLimitSnapshot struct {
	Primary   *rpcRateLimitWindow `json:"primary"`
	Secondary *rpcRateLimitWindow `json:"secondary"`
	Credits   *rpcCreditsSnapshot `json:"credits"`
	PlanType  string              `json:"planType"`
}

type rpcRateLimitWindow struct {
	UsedPercent        flexibleFloat `json:"usedPercent"`
	WindowDurationMins *int          `json:"windowDurationMins"`
	ResetsAt           *int64        `json:"resetsAt"`
}

type rpcCreditsSnapshot struct {
	HasCredits bool           `json:"hasCredits"`
	Unlimited  bool           `json:"unlimited"`
	Balance    *flexibleFloat `json:"balance"`
}

type rpcAccountResponse struct {
	Account *rpcAccount `json:"account"`
}

type rpcAccount struct {
	Type     string `json:"type"`
	Email    string `json:"email"`
	PlanType string `json:"planType"`
}

func windowFromRPC(input *rpcRateLimitWindow, now time.Time) *Window {
	if input == nil {
		return nil
	}
	var resetsAt *time.Time
	if input.ResetsAt != nil && *input.ResetsAt > 0 {
		reset := time.Unix(*input.ResetsAt, 0)
		resetsAt = &reset
	}
	return ptr(makeWindow(input.UsedPercent.value(), input.WindowDurationMins, resetsAt, now))
}

func accountFromRPC(response rpcAccountResponse, planFallback string) *Account {
	if response.Account == nil {
		if strings.TrimSpace(planFallback) == "" {
			return nil
		}
		return &Account{Plan: strings.TrimSpace(planFallback)}
	}
	account := &Account{}
	if strings.EqualFold(response.Account.Type, "chatgpt") {
		account.Email = strings.TrimSpace(response.Account.Email)
		account.Plan = firstNonEmpty(response.Account.PlanType, planFallback)
	} else {
		account.Plan = firstNonEmpty(planFallback, response.Account.Type)
	}
	if account.Email == "" && account.Plan == "" {
		return nil
	}
	return account
}

func creditsFromRPC(input *rpcCreditsSnapshot) *Credits {
	if input == nil {
		return nil
	}
	credits := &Credits{
		HasCredits: input.HasCredits,
		Unlimited:  input.Unlimited,
	}
	if input.Balance != nil {
		credits.Balance = ptr(input.Balance.value())
	}
	return credits
}

func recoverSnapshotFromRPCError(err error, now time.Time) (*Snapshot, error) {
	body := extractJSONAfter("body=", err.Error())
	if body == "" {
		return nil, err
	}
	var payload usageResponse
	if json.Unmarshal([]byte(body), &payload) != nil {
		return nil, err
	}
	snapshot := snapshotFromUsageResponse(payload, credentials{}, now)
	finalized, finalErr := finalizeSnapshot(snapshot, SourceCLI, now)
	if finalErr != nil {
		return nil, err
	}
	return finalized, nil
}

func extractJSONAfter(marker, text string) string {
	idx := strings.Index(text, marker)
	if idx < 0 {
		return ""
	}
	suffix := text[idx+len(marker):]
	start := strings.IndexByte(suffix, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for i, r := range suffix[start:] {
		if inString {
			if escaped {
				escaped = false
			} else if r == '\\' {
				escaped = true
			} else if r == '"' {
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return suffix[start : start+i+1]
			}
		}
	}
	return ""
}

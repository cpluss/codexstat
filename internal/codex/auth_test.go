package codex

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestParseCredentialsSupportsSnakeCaseTokens(t *testing.T) {
	data := []byte(`{
	  "tokens": {
	    "access_token": "access",
	    "refresh_token": "refresh",
	    "id_token": "id",
	    "account_id": "account"
	  },
	  "last_refresh": "2026-05-01T12:00:00Z"
	}`)

	creds, err := parseCredentials(data)
	if err != nil {
		t.Fatal(err)
	}
	if creds.AccessToken != "access" || creds.RefreshToken != "refresh" || creds.AccountID != "account" {
		t.Fatalf("unexpected credentials: %#v", creds)
	}
	if creds.LastRefresh == nil || creds.LastRefresh.Format(time.RFC3339) != "2026-05-01T12:00:00Z" {
		t.Fatalf("unexpected last refresh: %v", creds.LastRefresh)
	}
}

func TestAccountFromCredentialsReadsJWTClaims(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"email": "me@example.com",
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_plan_type":  "pro",
			"chatgpt_account_id": "acct_123",
		},
	})
	token := "header." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"

	account := accountFromCredentials(credentials{IDToken: token}, "")
	if account == nil {
		t.Fatal("expected account")
	}
	if account.Email != "me@example.com" || account.Plan != "pro" || account.AccountID != "acct_123" {
		t.Fatalf("unexpected account: %#v", account)
	}
}

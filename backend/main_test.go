package main

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestQwenHeadersIncludeCookieWhenProvided(t *testing.T) {
	headers := qwenHeaders("token-123", "foo=bar; baz=qux")

	if got := headers.Get("Authorization"); got != "Bearer token-123" {
		t.Fatalf("Authorization = %q, want %q", got, "Bearer token-123")
	}
	if got := headers.Get("Cookie"); got != "foo=bar; baz=qux" {
		t.Fatalf("Cookie = %q, want %q", got, "foo=bar; baz=qux")
	}
}

func TestQwenHeadersOmitCookieWhenEmpty(t *testing.T) {
	headers := qwenHeaders("token-123", "")

	if got := headers.Get("Cookie"); got != "" {
		t.Fatalf("Cookie = %q, want empty", got)
	}
}

func TestRunCompletionNonStreamParsesJSONBody(t *testing.T) {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/chats/new":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"success":true,"data":{"id":"chat-1"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/chat/completions":
			raw, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(raw), `"stream":false`) {
				t.Fatalf("completion payload missing stream=false: %s", string(raw))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"message":{"content":[{"text":"QWEN2API-LIVE-OK"}]}}`)
		default:
			t.Fatalf("unexpected upstream request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	settings := Settings{MaxInflightPerAccount: 1}
	store := NewJSONStore(filepath.Join(t.TempDir(), "accounts.json"), []any{})
	pool := NewAccountPool(store, settings, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := pool.Add(Account{Email: "test@example.com", Token: "token-123", StatusCode: "valid"}); err != nil {
		t.Fatalf("Add account: %v", err)
	}
	client := NewQwenClient(pool, settings, nil)
	client.http.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, strings.TrimPrefix(server.URL, "https://"))
		},
	}
	app := &App{
		settings: settings,
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		accounts: pool,
		client:   client,
		chatPool: NewChatIDPool(client, pool, settings, nil),
	}

	result, err := app.runCompletion(context.Background(), StandardRequest{
		Prompt:        "Reply with exactly: QWEN2API-LIVE-OK",
		ResolvedModel: "qwen3.6-plus",
		ResponseModel: "gpt-4o",
		ChatType:      "t2t",
	}, "")
	if err != nil {
		t.Fatalf("runCompletion error: %v", err)
	}
	if got := result.AnswerText; got != "QWEN2API-LIVE-OK" {
		t.Fatalf("AnswerText = %q, want %q", got, "QWEN2API-LIVE-OK")
	}
}

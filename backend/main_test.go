package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestEvaluateBrowserFetchUsesAbortTimeoutAndParsesSuccess(t *testing.T) {
	t.Helper()
	var (
		gotExpr string
		gotArg  map[string]any
	)
	evaluator := func(expr string, arg ...any) (any, error) {
		gotExpr = expr
		if len(arg) != 1 {
			t.Fatalf("arg len = %d, want 1", len(arg))
		}
		payload, ok := arg[0].(map[string]any)
		if !ok {
			t.Fatalf("arg type = %T, want map[string]any", arg[0])
		}
		gotArg = payload
		return map[string]any{
			"ok":     true,
			"status": float64(200),
			"body":   `{"success":true}`,
			"url":    "https://chat.qwen.ai/api/v2/chats/new",
		}, nil
	}

	status, body, err := evaluateBrowserFetch(
		context.Background(),
		evaluator,
		http.MethodPost,
		"/api/v2/chats/new",
		map[string]any{"foo": "bar"},
		"application/json",
		"token-123",
		1500*time.Millisecond,
	)
	if err != nil {
		t.Fatalf("evaluateBrowserFetch error: %v", err)
	}
	if status != 200 {
		t.Fatalf("status = %d, want 200", status)
	}
	if body != `{"success":true}` {
		t.Fatalf("body = %q, want success payload", body)
	}
	if !strings.Contains(gotExpr, "AbortController") {
		t.Fatalf("expression missing AbortController: %s", gotExpr)
	}
	if !strings.Contains(gotExpr, "controller.abort") {
		t.Fatalf("expression missing controller.abort: %s", gotExpr)
	}
	if got := gotArg["timeoutMs"]; got != 1500 {
		t.Fatalf("timeoutMs = %#v, want 1500", got)
	}
	if got := gotArg["path"]; got != "/api/v2/chats/new" {
		t.Fatalf("path = %#v, want /api/v2/chats/new", got)
	}
}

func TestEvaluateBrowserFetchReportsAbortTimeout(t *testing.T) {
	t.Helper()
	evaluator := func(expr string, arg ...any) (any, error) {
		return map[string]any{
			"ok":        false,
			"error":     "browser fetch timeout after 30000ms",
			"name":      "AbortError",
			"timed_out": true,
			"lastStage": "fetch_error",
			"trace": []any{
				map[string]any{"stage": "created", "elapsedMs": float64(0)},
				map[string]any{"stage": "fetch_error", "elapsedMs": float64(12), "error": "The operation was aborted."},
			},
		}, nil
	}

	status, body, err := evaluateBrowserFetch(
		context.Background(),
		evaluator,
		http.MethodPost,
		"/api/v2/chats/new",
		map[string]any{"foo": "bar"},
		"application/json",
		"token-123",
		0,
	)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if status != 0 {
		t.Fatalf("status = %d, want 0", status)
	}
	if body != "" {
		t.Fatalf("body = %q, want empty", body)
	}
	if !strings.Contains(err.Error(), "30000ms") {
		t.Fatalf("error = %q, want timeout detail", err)
	}
	if !strings.Contains(err.Error(), "stage=fetch_error") {
		t.Fatalf("error = %q, want stage detail", err)
	}
}

func TestEvaluateBrowserFetchReturnsContextTimeoutWhenEvaluateHangs(t *testing.T) {
	t.Helper()
	evaluator := func(expr string, arg ...any) (any, error) {
		select {}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	status, body, err := evaluateBrowserFetch(
		ctx,
		evaluator,
		http.MethodPost,
		"/api/v2/chats/new",
		map[string]any{"foo": "bar"},
		"application/json",
		"token-123",
		30*time.Millisecond,
	)
	if err == nil {
		t.Fatal("expected context timeout error, got nil")
	}
	if status != 0 {
		t.Fatalf("status = %d, want 0", status)
	}
	if body != "" {
		t.Fatalf("body = %q, want empty", body)
	}
	if !strings.Contains(err.Error(), "browser evaluate timeout") {
		t.Fatalf("error = %q, want evaluate timeout detail", err)
	}
}

func TestEvaluateBrowserFetchReturnsContextTimeoutWhenPollHangs(t *testing.T) {
	t.Helper()
	call := 0
	evaluator := func(expr string, arg ...any) (any, error) {
		call++
		if call == 1 {
			return true, nil
		}
		select {}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	status, body, err := evaluateBrowserFetch(
		ctx,
		evaluator,
		http.MethodPost,
		"/api/v2/chats/new",
		map[string]any{"foo": "bar"},
		"application/json",
		"token-123",
		30*time.Millisecond,
	)
	if err == nil {
		t.Fatal("expected context timeout error, got nil")
	}
	if status != 0 {
		t.Fatalf("status = %d, want 0", status)
	}
	if body != "" {
		t.Fatalf("body = %q, want empty", body)
	}
	if !strings.Contains(err.Error(), "browser evaluate timeout") {
		t.Fatalf("error = %q, want evaluate timeout detail", err)
	}
}

func TestEvaluateBrowserFetchReturnsTraceWhenJobNeverCompletes(t *testing.T) {
	t.Helper()
	call := 0
	evaluator := func(expr string, arg ...any) (any, error) {
		call++
		if call == 1 {
			return true, nil
		}
		return map[string]any{
			"done":      false,
			"lastStage": "fetch_started",
			"trace": []any{
				map[string]any{"stage": "created", "elapsedMs": float64(0)},
				map[string]any{"stage": "fetch_started", "elapsedMs": float64(5)},
			},
		}, nil
	}

	status, body, err := evaluateBrowserFetch(
		context.Background(),
		evaluator,
		http.MethodPost,
		"/api/v2/chats/new",
		map[string]any{"foo": "bar"},
		"application/json",
		"token-123",
		30*time.Millisecond,
	)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if status != 0 {
		t.Fatalf("status = %d, want 0", status)
	}
	if body != "" {
		t.Fatalf("body = %q, want empty", body)
	}
	if !strings.Contains(err.Error(), "stage=fetch_started") {
		t.Fatalf("error = %q, want stage detail", err)
	}
	if !strings.Contains(err.Error(), "trace=created@0ms -> fetch_started@5ms") {
		t.Fatalf("error = %q, want trace detail", err)
	}
}

func TestRunBrowserStepReturnsContextTimeoutWhenActionHangs(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := runBrowserStep(ctx, 30*time.Millisecond, "goto", func() error {
		select {}
	})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "browser goto timeout") {
		t.Fatalf("error = %q, want goto timeout detail", err)
	}
}

func TestLoadSettingsReadsSharedBrowserEnv(t *testing.T) {
	t.Setenv("BROWSER_MODE", "shared_cdp")
	t.Setenv("BROWSER_CDP_URL", "http://127.0.0.1:9222")

	settings := LoadSettings()

	if settings.BrowserMode != "shared_cdp" {
		t.Fatalf("BrowserMode = %q, want shared_cdp", settings.BrowserMode)
	}
	if settings.BrowserCDPURL != "http://127.0.0.1:9222" {
		t.Fatalf("BrowserCDPURL = %q, want shared URL", settings.BrowserCDPURL)
	}
}

func TestFindSharedQwenPageURLPrefersChatQwenAI(t *testing.T) {
	urls := []string{
		"https://www.google.com/",
		"https://chat.qwen.ai/c/123",
		"https://example.com/",
	}

	got := findSharedQwenPageURL(urls)
	if got != "https://chat.qwen.ai/c/123" {
		t.Fatalf("findSharedQwenPageURL() = %q, want chat.qwen.ai page", got)
	}
}

func TestShouldNavigateSharedBrowserPage(t *testing.T) {
	if shouldNavigateSharedBrowserPage("shared_cdp", "https://chat.qwen.ai/c/123") {
		t.Fatal("shared qwen page should not navigate again")
	}
	if !shouldNavigateSharedBrowserPage("embedded", "") {
		t.Fatal("embedded mode should navigate")
	}
}

func TestSharedBrowserFetchResponseURL(t *testing.T) {
	if got := sharedBrowserFetchResponseURL("/api/v2/chats/new"); got != "https://chat.qwen.ai/api/v2/chats/new" {
		t.Fatalf("sharedBrowserFetchResponseURL(relative) = %q", got)
	}
	if got := sharedBrowserFetchResponseURL("api/v2/chats/new"); got != "https://chat.qwen.ai/api/v2/chats/new" {
		t.Fatalf("sharedBrowserFetchResponseURL(no-leading-slash) = %q", got)
	}
	if got := sharedBrowserFetchResponseURL("https://chat.qwen.ai/api/v2/chats/new?chat_id=1"); got != "https://chat.qwen.ai/api/v2/chats/new?chat_id=1" {
		t.Fatalf("sharedBrowserFetchResponseURL(absolute) = %q", got)
	}
}

func TestParseSharedCDPRequestEvent(t *testing.T) {
	event := parseSharedCDPRequestEvent(map[string]any{
		"requestId": "123.45",
		"request": map[string]any{
			"method": "post",
			"url":    "https://chat.qwen.ai/api/v2/chats/new",
		},
	})

	if event.RequestID != "123.45" {
		t.Fatalf("RequestID = %q, want 123.45", event.RequestID)
	}
	if event.Method != "POST" {
		t.Fatalf("Method = %q, want POST", event.Method)
	}
	if event.URL != "https://chat.qwen.ai/api/v2/chats/new" {
		t.Fatalf("URL = %q, want target URL", event.URL)
	}
}

func TestMatchesSharedCDPRequest(t *testing.T) {
	event := sharedCDPRequestEvent{
		RequestID: "123.45",
		Method:    "POST",
		URL:       "https://chat.qwen.ai/api/v2/chats/new",
	}
	if !matchesSharedCDPRequest(event, http.MethodPost, "https://chat.qwen.ai/api/v2/chats/new") {
		t.Fatal("expected exact method and URL to match")
	}
	if matchesSharedCDPRequest(event, http.MethodGet, "https://chat.qwen.ai/api/v2/chats/new") {
		t.Fatal("GET should not match POST event")
	}
	if matchesSharedCDPRequest(event, http.MethodPost, "https://chat.qwen.ai/api/v2/chat/completions") {
		t.Fatal("different URL should not match")
	}
}

func TestParseSharedCDPResponseEvent(t *testing.T) {
	event := parseSharedCDPResponseEvent(map[string]any{
		"requestId": "123.45",
		"response": map[string]any{
			"status": float64(200),
			"url":    "https://chat.qwen.ai/api/v2/chats/new",
		},
	})

	if event.RequestID != "123.45" {
		t.Fatalf("RequestID = %q, want 123.45", event.RequestID)
	}
	if event.Status != 200 {
		t.Fatalf("Status = %d, want 200", event.Status)
	}
	if event.URL != "https://chat.qwen.ai/api/v2/chats/new" {
		t.Fatalf("URL = %q, want target URL", event.URL)
	}
}

func TestDecodeSharedCDPResponseBodyPlain(t *testing.T) {
	body, err := decodeSharedCDPResponseBody(map[string]any{
		"body":          `{"success":true}`,
		"base64Encoded": false,
	})
	if err != nil {
		t.Fatalf("decodeSharedCDPResponseBody() error = %v", err)
	}
	if body != `{"success":true}` {
		t.Fatalf("body = %q, want JSON payload", body)
	}
}

func TestDecodeSharedCDPResponseBodyBase64(t *testing.T) {
	body, err := decodeSharedCDPResponseBody(map[string]any{
		"body":          base64.StdEncoding.EncodeToString([]byte(`{"success":true}`)),
		"base64Encoded": true,
	})
	if err != nil {
		t.Fatalf("decodeSharedCDPResponseBody() error = %v", err)
	}
	if body != `{"success":true}` {
		t.Fatalf("body = %q, want decoded JSON payload", body)
	}
}

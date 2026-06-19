package main

import "testing"

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

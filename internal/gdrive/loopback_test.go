package gdrive

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestLoopbackServer_CapturesAuthCode(t *testing.T) {
	srv, err := NewLoopbackServer()
	if err != nil {
		t.Fatalf("NewLoopbackServer() error: %v", err)
	}

	// The server should provide a redirect URL with port.
	redirectURL := srv.RedirectURL()
	if redirectURL == "" {
		t.Fatal("RedirectURL() returned empty string")
	}

	// Simulate Google redirecting with an auth code.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		resp, err := http.Get(redirectURL + "?code=test-auth-code-123&state=state-token")
		if err != nil {
			// Server may close before response completes; not a test failure.
			return
		}
		_ = resp.Body.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	code, err := srv.WaitForCode(ctx)
	if err != nil {
		t.Fatalf("WaitForCode() error: %v", err)
	}
	if code != "test-auth-code-123" {
		t.Errorf("WaitForCode() = %q, want %q", code, "test-auth-code-123")
	}

	srv.Close()
	wg.Wait()
}

func TestLoopbackServer_ReturnsErrorOnMissingCode(t *testing.T) {
	srv, err := NewLoopbackServer()
	if err != nil {
		t.Fatalf("NewLoopbackServer() error: %v", err)
	}

	// Simulate redirect with an error instead of a code.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		resp, err := http.Get(srv.RedirectURL() + "?error=access_denied")
		if err != nil {
			// Server may close before response completes; not a test failure.
			return
		}
		_ = resp.Body.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = srv.WaitForCode(ctx)
	if err == nil {
		t.Fatal("WaitForCode() expected error for error response, got nil")
	}

	srv.Close()
	wg.Wait()
}

func TestLoopbackServer_TimesOutWhenNoCallback(t *testing.T) {
	srv, err := NewLoopbackServer()
	if err != nil {
		t.Fatalf("NewLoopbackServer() error: %v", err)
	}
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = srv.WaitForCode(ctx)
	if err == nil {
		t.Fatal("WaitForCode() expected timeout error, got nil")
	}
}

func TestLoopbackServer_RedirectURLFormat(t *testing.T) {
	srv, err := NewLoopbackServer()
	if err != nil {
		t.Fatalf("NewLoopbackServer() error: %v", err)
	}
	defer srv.Close()

	url := srv.RedirectURL()
	// Should be http://localhost:<port>/callback
	want := fmt.Sprintf("http://localhost:%d/callback", srv.Port())
	if url != want {
		t.Errorf("RedirectURL() = %q, want %q", url, want)
	}
}

package gdrive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSetRedirectURL_OverridesCredentialRedirect(t *testing.T) {
	// Create a temporary credentials file with a "web" type (wrong for CLI).
	creds := map[string]interface{}{
		"web": map[string]interface{}{
			"client_id":     "test-client-id",
			"client_secret": "test-client-secret",
			"auth_uri":      "https://accounts.google.com/o/oauth2/auth",
			"token_uri":     "https://oauth2.googleapis.com/token",
			"redirect_uris": []string{"https://example.cloudflareaccess.com/callback"},
		},
	}
	data, err := json.Marshal(creds)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(credPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := NewOAuthConfig(credPath)
	if err != nil {
		t.Fatalf("NewOAuthConfig() error: %v", err)
	}

	// Before override, redirect should be the Cloudflare one.
	if cfg.RedirectURL != "https://example.cloudflareaccess.com/callback" {
		t.Fatalf("initial RedirectURL = %q, want Cloudflare URL", cfg.RedirectURL)
	}

	// Override to localhost.
	cfg.SetRedirectURL("http://localhost:12345/callback")

	if cfg.RedirectURL != "http://localhost:12345/callback" {
		t.Errorf("after SetRedirectURL, RedirectURL = %q, want %q", cfg.RedirectURL, "http://localhost:12345/callback")
	}

	// The auth URL should use the overridden redirect.
	authURL := cfg.GetAuthURL()
	if !containsSubstring(authURL, "redirect_uri=http") {
		t.Errorf("GetAuthURL() = %q, should contain localhost redirect", authURL)
	}
	if containsSubstring(authURL, "cloudflareaccess") {
		t.Errorf("GetAuthURL() = %q, should NOT contain cloudflare redirect", authURL)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && searchSubstring(s, sub)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

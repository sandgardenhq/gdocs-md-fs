package gdrive

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// OAuth scopes required by the application.
var oauthScopes = []string{
	"https://www.googleapis.com/auth/drive.file",
	"https://www.googleapis.com/auth/drive.readonly",
	"https://www.googleapis.com/auth/documents",
}

// OAuthConfig holds OAuth 2.0 client credentials and provides methods to
// perform the authorization flow.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string

	cfg *oauth2.Config
}

// TokenStore reads and writes OAuth tokens to disk.
type TokenStore struct {
	path string
}

// configDir returns the configuration directory for gdocs-md.
// It prefers $XDG_CONFIG_HOME/gdocs-md/ and falls back to ~/.config/gdocs-md/.
func configDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "gdocs-md")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// Last resort fallback.
		return filepath.Join(".config", "gdocs-md")
	}
	return filepath.Join(home, ".config", "gdocs-md")
}

// NewTokenStore returns a TokenStore that persists tokens to the standard
// config directory at configDir()/token.json.
func NewTokenStore() *TokenStore {
	return &TokenStore{
		path: filepath.Join(configDir(), "token.json"),
	}
}

// SaveToken writes the OAuth token to disk as JSON. Parent directories are
// created if they do not exist.
func (ts *TokenStore) SaveToken(token *oauth2.Token) error {
	dir := filepath.Dir(ts.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("gdrive: create config directory: %w", err)
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("gdrive: marshal token: %w", err)
	}

	if err := os.WriteFile(ts.path, data, 0600); err != nil {
		return fmt.Errorf("gdrive: write token file: %w", err)
	}
	return nil
}

// LoadToken reads a previously saved OAuth token from disk.
func (ts *TokenStore) LoadToken() (*oauth2.Token, error) {
	data, err := os.ReadFile(ts.path)
	if err != nil {
		return nil, fmt.Errorf("gdrive: read token file: %w", err)
	}

	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("gdrive: unmarshal token: %w", err)
	}
	return &token, nil
}

// credentialsFile is the expected JSON structure of the client credentials file
// downloaded from the Google Cloud console.
type credentialsFile struct {
	Installed *credentialsInstalled `json:"installed"`
	Web       *credentialsInstalled `json:"web"`
}

type credentialsInstalled struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURIs []string `json:"redirect_uris"`
}

// NewOAuthConfig loads client credentials from a JSON file (typically
// configDir()/credentials.json) and returns an OAuthConfig ready for the
// authorization flow. If credentialsPath is empty, the default location
// configDir()/credentials.json is used.
func NewOAuthConfig(credentialsPath string) (*OAuthConfig, error) {
	if credentialsPath == "" {
		credentialsPath = filepath.Join(configDir(), "credentials.json")
	}

	data, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("gdrive: read credentials file %s: %w", credentialsPath, err)
	}

	cfg, err := google.ConfigFromJSON(data, oauthScopes...)
	if err != nil {
		return nil, fmt.Errorf("gdrive: parse credentials: %w", err)
	}

	return &OAuthConfig{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		cfg:          cfg,
	}, nil
}

// GetAuthURL returns the URL the user must visit to authorize the application.
func (oc *OAuthConfig) GetAuthURL() string {
	return oc.cfg.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
}

// Exchange trades an authorization code for an OAuth token.
func (oc *OAuthConfig) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	token, err := oc.cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("gdrive: exchange auth code: %w", err)
	}
	return token, nil
}

// TokenSource returns a reusable, auto-refreshing token source from the given
// token. The returned source automatically refreshes the token when it expires.
func (oc *OAuthConfig) TokenSource(ctx context.Context, token *oauth2.Token) oauth2.TokenSource {
	return oc.cfg.TokenSource(ctx, token)
}

// Config returns the underlying oauth2.Config for advanced use cases.
func (oc *OAuthConfig) Config() *oauth2.Config {
	return oc.cfg
}

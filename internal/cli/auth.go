package cli

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/brittcrawford/gdocs-md/internal/gdrive"
	"github.com/spf13/cobra"
)

// openBrowser attempts to open the given URL in the default browser.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
}

func newAuthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "auth",
		Short: "Authenticate with Google Drive via OAuth",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load OAuth config from credentials file.
			oauthCfg, err := gdrive.NewOAuthConfig("")
			if err != nil {
				return fmt.Errorf("failed to load OAuth credentials: %w", err)
			}

			// Start a local server to receive the OAuth callback.
			srv, err := gdrive.NewLoopbackServer()
			if err != nil {
				return fmt.Errorf("failed to start local auth server: %w", err)
			}
			defer srv.Close()

			// Override the redirect URL to point to our local server,
			// regardless of what the credentials file specifies.
			oauthCfg.SetRedirectURL(srv.RedirectURL())

			// Generate auth URL and open the browser.
			authURL := oauthCfg.GetAuthURL()
			fmt.Println("Opening browser for Google authorization...")
			fmt.Println()
			fmt.Println("If the browser doesn't open, visit this URL:")
			fmt.Println()
			fmt.Println("  " + authURL)
			fmt.Println()

			_ = openBrowser(authURL)

			fmt.Println("Waiting for authorization...")

			// Wait for the callback with a generous timeout.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			code, err := srv.WaitForCode(ctx)
			if err != nil {
				return fmt.Errorf("failed to receive authorization: %w", err)
			}

			// Exchange auth code for token.
			token, err := oauthCfg.Exchange(context.Background(), code)
			if err != nil {
				return fmt.Errorf("failed to exchange authorization code: %w", err)
			}

			// Save token to config directory.
			store := gdrive.NewTokenStore()
			if err := store.SaveToken(token); err != nil {
				return fmt.Errorf("failed to save token: %w", err)
			}

			fmt.Println("Authentication successful! Token saved.")
			return nil
		},
	}
}

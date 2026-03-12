package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/brittcrawford/gdocs-md/internal/gdrive"
	"github.com/spf13/cobra"
)

func newAuthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "auth",
		Short: "Authenticate with Google Drive via OAuth",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load or create OAuth config from credentials file.
			oauthCfg, err := gdrive.NewOAuthConfig("")
			if err != nil {
				return fmt.Errorf("failed to load OAuth credentials: %w", err)
			}

			// Generate auth URL and prompt the user.
			authURL := oauthCfg.GetAuthURL()
			fmt.Println("Visit the following URL to authorize gdocs-md:")
			fmt.Println()
			fmt.Println("  " + authURL)
			fmt.Println()
			fmt.Print("Paste the authorization code here: ")

			reader := bufio.NewReader(os.Stdin)
			code, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read authorization code: %w", err)
			}
			code = strings.TrimSpace(code)

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

package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/brittcrawford/gdocs-md/internal/gdrive"
	"github.com/brittcrawford/gdocs-md/internal/ragfs"
	"github.com/spf13/cobra"
)

func newMountCmd() *cobra.Command {
	var (
		cacheSize  string
		cacheTTL   time.Duration
		metaTTL    time.Duration
		foreground bool
		readOnly   bool
	)

	cmd := &cobra.Command{
		Use:     "mount <folder-id> <mountpoint>",
		Short:   "Mount a Google Drive folder as a local directory",
		Args:    cobra.ExactArgs(2),
		Example: "  gdocs-md mount abc123 ~/drive",
		RunE: func(cmd *cobra.Command, args []string) error {
			folderID := args[0]
			mountpoint := args[1]

			// Parse cache size.
			maxBytes, err := parseSize(cacheSize)
			if err != nil {
				return fmt.Errorf("invalid --cache-size value %q: %w", cacheSize, err)
			}

			// Load OAuth token.
			store := gdrive.NewTokenStore()
			token, err := store.LoadToken()
			if err != nil {
				return fmt.Errorf("failed to load token (run 'gdocs-md auth' first): %w", err)
			}

			// Create OAuth config and token source.
			oauthCfg, err := gdrive.NewOAuthConfig("")
			if err != nil {
				return fmt.Errorf("failed to load OAuth credentials: %w", err)
			}

			ctx := context.Background()
			ts := oauthCfg.TokenSource(ctx, token)

			// Create Google Drive client with root folder ID.
			driveClient, err := gdrive.NewClient(ctx, ts, folderID)
			if err != nil {
				return fmt.Errorf("failed to create Drive client: %w", err)
			}

			handler := gdrive.NewDriveHandler(driveClient, folderID)
			server := ragfs.NewServer(handler, mountpoint,
				ragfs.WithCacheOptions(
					ragfs.WithMaxSize(maxBytes),
					ragfs.WithContentTTL(cacheTTL),
					ragfs.WithMetaTTL(metaTTL),
				),
				ragfs.WithReadOnly(readOnly),
			)

			// Set up signal handling for graceful unmount.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				sig := <-sigCh
				fmt.Fprintf(os.Stderr, "\nReceived %s, unmounting...\n", sig)
				if err := server.Unmount(); err != nil {
					fmt.Fprintf(os.Stderr, "Error unmounting: %v\n", err)
				}
			}()

			if foreground {
				fmt.Fprintf(os.Stderr, "Mounting %s at %s (foreground)\n", folderID, mountpoint)
			} else {
				fmt.Fprintf(os.Stderr, "Mounting %s at %s\n", folderID, mountpoint)
			}

			// Mount and serve (blocks until unmounted).
			if err := server.Mount(); err != nil {
				return fmt.Errorf("mount failed: %w", err)
			}

			fmt.Fprintln(os.Stderr, "Unmounted successfully.")
			return nil
		},
	}

	cmd.Flags().StringVar(&cacheSize, "cache-size", "100MB", "Maximum in-memory cache size")
	cmd.Flags().DurationVar(&cacheTTL, "cache-ttl", 60*time.Second, "Cache TTL for content")
	cmd.Flags().DurationVar(&metaTTL, "meta-ttl", 30*time.Second, "Cache TTL for metadata")
	cmd.Flags().BoolVar(&foreground, "foreground", false, "Run in foreground")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "Mount as read-only")

	return cmd
}

// parseSize parses a human-readable size string (e.g. "100MB", "1GB", "50mb")
// into a byte count. Supported suffixes: B, KB, MB, GB, TB (case-insensitive).
// A plain number without a suffix is treated as bytes.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}

	s = strings.ToUpper(s)

	multipliers := []struct {
		suffix string
		mult   int64
	}{
		{"TB", 1 << 40},
		{"GB", 1 << 30},
		{"MB", 1 << 20},
		{"KB", 1 << 10},
		{"B", 1},
	}

	for _, m := range multipliers {
		if strings.HasSuffix(s, m.suffix) {
			numStr := strings.TrimSpace(s[:len(s)-len(m.suffix)])
			n, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid number %q: %w", numStr, err)
			}
			return int64(n * float64(m.mult)), nil
		}
	}

	// No suffix — treat as bytes.
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	return n, nil
}

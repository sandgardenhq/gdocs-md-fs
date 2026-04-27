package cli

import (
	"github.com/spf13/cobra"
)

var rootCmd *cobra.Command

var verbose bool

// NewRootCmd creates and returns the root cobra command with all subcommands
// registered.
func NewRootCmd() *cobra.Command {
	rootCmd = &cobra.Command{
		Use:   "gdocs-md-fs",
		Short: "Mount Google Drive as a local filesystem with Docs as Markdown",
	}

	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")

	rootCmd.AddCommand(newAuthCmd())
	rootCmd.AddCommand(newMountCmd())
	rootCmd.AddCommand(newVersionCmd())

	return rootCmd
}

// Execute initializes the root command and runs it.
func Execute() error {
	rootCmd = NewRootCmd()
	return rootCmd.Execute()
}

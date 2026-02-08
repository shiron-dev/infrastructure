package cmd

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	debug   bool
)

var rootCmd = &cobra.Command{
	Use:           "cmt",
	Short:         "Compose Manage Tool — push-based sync for Docker Compose projects",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `cmt is a source-of-truth, push-based tool that syncs Docker Compose
project files from a local repository to remote hosts via SSH.

It follows a plan/apply workflow similar to Terraform:
  cmt plan   — show what would change
  cmt apply  — apply changes (with confirmation)`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if debug {
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})))
		} else {
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelWarn,
			})))
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "config.yml", "path to cmt config file")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:   "sitegen",
		Short: "Static site generator with optional live editing",
		Long:  "sitegen - converts markdown to browsable HTML with sidebar TOC, GFM tables, and security headers.",
	}

	// --- build -----------------------------------------------------------
	buildCmd := &cobra.Command{
		Use:   "build",
		Short: "Generate HTML from markdown and exit",
		RunE: func(cmd *cobra.Command, args []string) error {
			src, _ := cmd.Flags().GetString("src")
			out, _ := cmd.Flags().GetString("out")
			fmt.Println("Building site...")
			return runBuild(src, out)
		},
	}
	buildCmd.Flags().String("src", "content", "markdown source (file or directory)")
	buildCmd.Flags().String("out", "docs", "output directory")

	// --- serve -----------------------------------------------------------
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Generate, watch for changes, and serve over HTTP",
		RunE: func(cmd *cobra.Command, args []string) error {
			src, _ := cmd.Flags().GetString("src")
			out, _ := cmd.Flags().GetString("out")
			addr, _ := cmd.Flags().GetString("addr")
			noAddr, _ := cmd.Flags().GetBool("no-addr")
			write, _ := cmd.Flags().GetBool("write")
			verbose, _ := cmd.Flags().GetBool("verbose")

			addrSet := cmd.Flags().Changed("addr")

			if write && os.Getenv("WRITE_MODE") == "" {
				fmt.Fprintln(os.Stderr, "WARNING: write mode enabled via --write flag")
			}
			// Allow WRITE_MODE env var as alternative to --write flag
			if !write && os.Getenv("WRITE_MODE") == "true" {
				write = true
			}

			return runServe(src, out, addr, addrSet, noAddr, write, verbose)
		},
	}
	serveCmd.Flags().String("src", "content", "markdown source (file or directory)")
	serveCmd.Flags().String("out", "docs", "output directory")
	serveCmd.Flags().String("addr", ":8080", "listen address")
	serveCmd.Flags().Bool("no-addr", false, "skip TCP listener (overlay only)")
	serveCmd.Flags().Bool("write", false, "enable markdown editor (read-write mode)")
	serveCmd.Flags().Bool("verbose", false, "enable verbose logging")
	serveCmd.Flags().Duration("since", 720*time.Hour, "default sidebar age filter (Go duration, e.g. 720h for past month)")

	root.AddCommand(buildCmd, serveCmd)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

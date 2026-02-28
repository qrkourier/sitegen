package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "build":
		fs := flag.NewFlagSet("build", flag.ExitOnError)
		src := fs.String("src", "content", "markdown source (file or directory)")
		out := fs.String("out", "docs", "output directory")
		fs.Parse(os.Args[2:])
		fmt.Println("Building site...")
		if err := runBuild(*src, *out); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		src := fs.String("src", "content", "markdown source (file or directory)")
		out := fs.String("out", "docs", "output directory")
		addr := fs.String("addr", ":8080", "listen address")
		noAddr := fs.Bool("no-addr", false, "disable TCP listener (Ziti only)")
		verbose := fs.Bool("verbose", false, "enable verbose logging")
		fs.Parse(os.Args[2:])
		if err := runServe(*src, *out, *addr, *noAddr, *verbose); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `sitegen - static site generator

Usage:
  sitegen build [-src path] [-out dir]
  sitegen serve [-src path] [-out dir] [-addr :port] [-no-addr] [-verbose]

Commands:
  build    Generate HTML from markdown and exit
  serve    Generate, watch for changes, and serve over HTTP
`)
}

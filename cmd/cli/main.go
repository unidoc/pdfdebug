// Package main provides the pdfdebug CLI binary for PDF inspection.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
)

// version is overridden via -ldflags "-X main.version=x.y.z" in release builds.
var version = "dev"

func main() {
	// Suppress pdfcpu's internal log output so stderr stays clean for JSON errors.
	log.SetOutput(io.Discard)

	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "--help", "-h", "help":
		printUsage(os.Stderr)
		os.Exit(0)
	case "--version", "-v":
		fmt.Fprintf(os.Stdout, "pdfdebug version %s\n", version)
		os.Exit(0)
	case "dump":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: pdfdebug dump <resource> [flags] <file>")
			os.Exit(1)
		}
		resource := os.Args[2]
		remaining := os.Args[3:]
		switch resource {
		case "tree":
			os.Exit(runTreeDump(remaining))
		case "object":
			os.Exit(runObjectDump(remaining))
		case "stream":
			os.Exit(runStreamDump(remaining))
		default:
			fmt.Fprintf(os.Stderr, "Unknown resource: %s\n", resource)
			printUsage(os.Stderr)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage(os.Stderr)
		os.Exit(1)
	}
}

// printUsage writes the CLI help text to w.
func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: pdfdebug <command> [flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  dump tree [--json] [--depth N] <file>   Dump PDF object tree as JSON")
	fmt.Fprintln(w, "  dump object [--json] --ref \"N G R\" <file>  Dump a single PDF object")
	fmt.Fprintln(w, "  dump stream [--json] --page N <file>    Dump page content stream")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --help      Show this help message")
	fmt.Fprintln(w, "  --version   Show version information")
}

// writeJSONError writes a JSON error object to w.
func writeJSONError(w io.Writer, msg string) {
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// writeJSONWarning writes a JSON warning object to w.
func writeJSONWarning(w io.Writer, msg string) {
	json.NewEncoder(w).Encode(map[string]string{"warning": msg})
}

// parseDumpFlags creates a FlagSet for dump subcommands with common flags.
func parseDumpFlags(name string, args []string) (*flag.FlagSet, bool, int, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard) // suppress default flag error output
	jsonFlag := fs.Bool("json", false, "Output as JSON (default, always on)")
	depthFlag := fs.Int("depth", 0, "Max tree depth (0 = unlimited)")
	if err := fs.Parse(args); err != nil {
		return fs, false, 0, err
	}
	return fs, *jsonFlag, *depthFlag, nil
}

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// validateUsage is the one-line usage string for the validate command.
const validateUsage = "Usage: pdfdebug validate [--profile pdfa-1b|pdfua-1-structural] [--json] [--pretty] <file>"

// runValidate handles the top-level `validate` command: run the bounded
// structural conformance rule set for a profile and report problems. It uses a
// three-way exit contract distinct from the `dump` commands:
//
//	0  ran successfully, ZERO error-severity problems (warnings/info allowed)
//	1  ran successfully AND found >=1 error-severity problem (the CI gate)
//	2  operational error (missing/unreadable file, unknown profile, view failure)
//
// Exit 0 means "no structural errors found," NOT "compliant/valid".
func runValidate(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	profileFlag := fs.String("profile", pdfcore.ProfilePDFA1B, "Rule profile: pdfa-1b (default) or pdfua-1-structural")
	jsonFlag := fs.Bool("json", false, "Output structured JSON (default is human-readable plain text)")
	prettyFlag := fs.Bool("pretty", false, "Indent JSON output (no effect on plain text)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, validateUsage)
		return 2
	}

	profile := *profileFlag
	// An unknown profile is a usage error - list the valid profiles to
	// stderr, no partial run, operational exit (2).
	if !slices.Contains(pdfcore.ValidProfiles, profile) {
		fmt.Fprintf(os.Stderr, "unknown profile %q; valid profiles: %s\n", profile, strings.Join(pdfcore.ValidProfiles, ", "))
		return 2
	}

	filePath := fs.Arg(0)
	if filePath == "" {
		fmt.Fprintln(os.Stderr, validateUsage)
		return 2
	}
	return execValidate(filePath, profile, *jsonFlag, *prettyFlag)
}

// execValidate opens the PDF, runs the profile's rule set, and renders the
// result. The ErrEncryptedPDF open failure is reconciled into a single
// error-severity encryption problem (exit 1), not a bare operational
// error. Other open failures and view failures are operational (exit 2).
func execValidate(filePath, profile string, jsonOut, pretty bool) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("internal error: %v", r))
			exitCode = 2
		}
	}()

	ins := pdfcore.NewInspector()
	if _, err := ins.Open("cli", filePath); err != nil {
		if errors.Is(err, pdfcore.ErrEncryptedPDF) && profile == pdfcore.ProfilePDFA1B {
			// Reconciliation: no-encryption is a PDF/A-1b rule, and encrypted
			// docs never reach Validate as an open tab, so surface encryption as
			// an error problem here (exit 1). Only under the PDF/A profile - a
			// PDF/UA-structural run has no encryption rule and must never gate on
			// one, so an unopenable encrypted file there is operational (exit 2).
			return renderValidate(pdfcore.EncryptedResult(profile), jsonOut, pretty)
		}
		return handleOpenError(err) // operational: exit 2
	}
	defer func() { _ = ins.Close("cli") }()

	res, err := ins.Validate("cli", profile)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}
	return renderValidate(res, jsonOut, pretty)
}

// renderValidate writes the result (plain text default, --json opts in) and
// returns the compliance-gate exit code: 1 when any error-severity problem was
// found, else 0. It ALSO returns 1 (fail closed) when a gating-profile rule
// could not be evaluated (degraded to an info problem) - reporting exit 0 "no
// structural errors" for a file whose required rule never actually ran would
// silently pass a CI gate on an unverified document.
func renderValidate(res *pdfcore.ValidationResult, jsonOut, pretty bool) int {
	if jsonOut {
		if err := emit(os.Stdout, res, pretty); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
	} else if err := printValidatePlain(os.Stdout, res); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	if res.Summary.Errors > 0 {
		return 1
	}
	if res.Summary.Info > 0 && pdfcore.ProfileGates(res.Profile) {
		return 1
	}
	return 0
}

// severityOrder is the display order for the grouped plain-text problem list.
var severityOrder = []string{"error", "warning", "info"}

// printValidatePlain renders the grouped problem list plus a summary count.
// NON-CONTRACTUAL; use --json to parse. It always carries the not-authoritative
// disclaimer and never states an authoritative conformance verdict.
func printValidatePlain(out io.Writer, res *pdfcore.ValidationResult) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Validation profile: %s\n", res.Profile)
	fmt.Fprintf(&b, "%s\n\n", res.Disclaimer)

	if len(res.Problems) == 0 {
		b.WriteString("no structural problems found\n\n")
	} else {
		for _, sev := range severityOrder {
			group := problemsBySeverity(res.Problems, sev)
			if len(group) == 0 {
				continue
			}
			fmt.Fprintf(&b, "%s problems:\n", strings.ToUpper(sev))
			for _, p := range group {
				fmt.Fprintf(&b, "  [%s] %s\n", p.RuleID, p.Message)
				if p.ObjRef != "" {
					fmt.Fprintf(&b, "      object: %s  (%s)\n", p.ObjRef, p.ObjNodeID)
				} else {
					b.WriteString("      object: (document-level)\n")
				}
				fmt.Fprintf(&b, "      spec: %s\n", p.SpecRef)
			}
			b.WriteString("\n")
		}
	}

	fmt.Fprintf(&b, "Summary: %d %s, %d %s",
		res.Summary.Errors, plural(res.Summary.Errors, "error"),
		res.Summary.Warnings, plural(res.Summary.Warnings, "warning"))
	// Surface degraded (info) rules so a "0 errors, 0 warnings" line never hides
	// a rule that could not be evaluated.
	if res.Summary.Info > 0 {
		fmt.Fprintf(&b, ", %d %s", res.Summary.Info, plural(res.Summary.Info, "info problem"))
	}
	b.WriteString("\n")
	_, err := io.WriteString(out, b.String())
	return err
}

// problemsBySeverity filters problems to those matching sev.
func problemsBySeverity(ps []pdfcore.Problem, sev string) []pdfcore.Problem {
	out := make([]pdfcore.Problem, 0, len(ps))
	for _, p := range ps {
		if p.Severity == sev {
			out = append(out, p)
		}
	}
	return out
}

// plural returns word or its simple plural for n.
func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

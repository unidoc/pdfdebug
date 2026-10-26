package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// signatureExportFlags holds the export flags of `dump signatures`. An empty
// value means the flag was not given.
type signatureExportFlags struct {
	cmsPath  string
	certsDir string
	field    string
}

// runSignaturesDump handles `dump signatures`: enumerate + decompose every
// signature field. Plain-text default is a per-signature block;
// --json emits a top-level array. Zero signature fields is a normal empty
// state (exit 0); open or view errors go to stderr with exit 2. The output
// reports decomposed structural facts only - never a trust verdict, and no
// URL from a certificate is fetched. --export-cms and --export-certs write
// the signature's CMS and certificates to files before anything is printed.
func runSignaturesDump(args []string) int {
	usage := usageLine("dump signatures")

	fs := flag.NewFlagSet("dump signatures", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	prettyFlag := fs.Bool("pretty", false, "Indent JSON output (no effect on plain text)")
	jsonFlag := fs.Bool("json", false, "Output structured JSON (default is human-readable plain text)")
	cmsFlag := fs.String("export-cms", "", "Write the signature's CMS bytes, as found, to FILE")
	certsFlag := fs.String("export-certs", "", "Write each certificate as DER into DIR")
	fieldFlag := fs.String("field", "", "Signature field to export, by fully qualified name")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, usage)
		return 1
	}
	if !requirePositionals(fs, 1, usage) {
		return 1
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if set["export-cms"] && *cmsFlag == "" {
		writeJSONError(os.Stderr, "--export-cms needs a non-empty FILE")
		return 1
	}
	if set["export-certs"] && *certsFlag == "" {
		writeJSONError(os.Stderr, "--export-certs needs a non-empty DIR")
		return 1
	}
	if set["field"] && *fieldFlag == "" {
		writeJSONError(os.Stderr, "--field needs a non-empty NAME")
		return 1
	}
	if set["field"] && !set["export-cms"] && !set["export-certs"] {
		writeJSONError(os.Stderr, "--field selects the signature to export; use it with --export-cms or --export-certs")
		return 1
	}

	flags := docViewFlags{pretty: *prettyFlag, json: *jsonFlag}
	if !set["export-cms"] && !set["export-certs"] {
		return execSignaturesDump(fs.Arg(0), flags)
	}
	return execSignaturesExport(fs.Arg(0), flags, signatureExportFlags{
		cmsPath:  *cmsFlag,
		certsDir: *certsFlag,
		field:    *fieldFlag,
	})
}

// execSignaturesDump opens the PDF and writes the signature decomposition as
// plain-text blocks (default) or a JSON array (--json).
func execSignaturesDump(filePath string, flags docViewFlags) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("internal error: %v", r))
			exitCode = 2
		}
	}()

	ins, code := openSignaturesForCLI(filePath)
	if code != 0 {
		return code
	}
	defer func() { _ = ins.Close("cli") }()

	list, err := ins.GetSignatures("cli")
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}
	return printSignatures(list.Signatures, flags)
}

// execSignaturesExport opens the PDF, writes the requested exports, and then
// prints exactly what execSignaturesDump prints. On an export failure nothing
// goes to stdout.
func execSignaturesExport(filePath string, flags docViewFlags, ex signatureExportFlags) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("internal error: %v", r))
			exitCode = 2
		}
	}()

	ins, code := openSignaturesForCLI(filePath)
	if code != 0 {
		return code
	}
	defer func() { _ = ins.Close("cli") }()

	list, sb, err := ins.GetSignatureBytes("cli")
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}
	if code := runSignatureExports(filePath, list.Signatures, sb, ex); code != 0 {
		return code
	}
	return printSignatures(list.Signatures, flags)
}

// openSignaturesForCLI opens filePath for the signatures command and runs the
// afterCLIOpen seam.
func openSignaturesForCLI(filePath string) (*pdfcore.Inspector, int) {
	ins, _, code := openForCLI(filePath)
	if code != 0 {
		return nil, code
	}
	if afterCLIOpen != nil {
		afterCLIOpen(filePath)
	}
	return ins, 0
}

// printSignatures writes sigs as plain text or, with --json, as a JSON array.
func printSignatures(sigs []pdfcore.SignatureField, flags docViewFlags) int {
	if flags.json {
		// Emit the slice directly so the top-level shape is a JSON array.
		if err := emit(os.Stdout, sigs, flags.pretty); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}
	if err := printSignaturesPlain(os.Stdout, sigs); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}

// printSignaturesPlain renders one aligned key/value block per signature
// field: identity, signature-dict entries, signer/chain facts, and the
// ByteRange coverage measurement. NON-CONTRACTUAL; use --json to parse. It
// states coverage facts without implying breakage and never emits trust-claim
// language.
func printSignaturesPlain(out io.Writer, sigs []pdfcore.SignatureField) error {
	if len(sigs) == 0 {
		_, err := fmt.Fprintln(out, "no signature fields")
		return err
	}
	w := &kvWriter{}
	for i, s := range sigs {
		if i > 0 {
			w.Gap()
		}
		w.Heading(fmt.Sprintf("Signature field %d of %d", i+1, len(sigs)))
		w.Add("Field", asciiSafe(dashIfEmpty(s.FieldName)))
		if !s.Signed {
			w.Add("Status", "unsigned (placeholder field, no /V)")
			continue
		}
		w.Add("Status", "signed")
		if s.SignatureRef != "" {
			w.Add("Signature dict", s.SignatureRef)
		} else {
			w.Add("Signature dict", "(direct, inline in the field)")
		}
		w.Add("SubFilter", dashIfEmpty(s.SubFilter))
		w.Add("Type", s.Type)
		if s.SigningTimeRaw != "" {
			v := s.SigningTimeRaw
			if s.SigningTime != "" {
				v = s.SigningTime + " (raw " + s.SigningTimeRaw + ")"
			}
			w.Add("Signing time", asciiSafe(v))
		}
		if s.Name != "" {
			w.Add("Name", asciiSafe(s.Name))
		}
		if s.Reason != "" {
			w.Add("Reason", asciiSafe(s.Reason))
		}
		if s.Location != "" {
			w.Add("Location", asciiSafe(s.Location))
		}
		if s.ContactInfo != "" {
			w.Add("Contact", asciiSafe(s.ContactInfo))
		}
		if s.Signer != nil {
			w.Add("Signer", asciiSafe(s.Signer.Subject))
			w.Add("Issuer", asciiSafe(s.Signer.Issuer))
			w.Add("Serial", s.Signer.Serial)
			w.Add("Validity", s.Signer.NotBefore+" to "+s.Signer.NotAfter)
		} else if s.DecomposeError == "" {
			// Reached only for signed entries (the loop `continue`s on unsigned).
			w.Add("Signer", "(not identified)")
		}
		if s.DigestAlgorithm != "" {
			w.Add("Digest algorithm", s.DigestAlgorithm)
		}
		if s.SignatureAlgorithm != "" {
			w.Add("Signature algorithm", s.SignatureAlgorithm)
		}
		if s.CMSEncoding != "" {
			enc := s.CMSEncoding
			if enc == "BER" {
				enc += " (normalized to DER for parsing)"
			}
			w.Add("CMS encoding", enc)
		}
		if t := s.CMSTrailingBytes; t != nil {
			verb := "contain"
			if t.Count == 1 {
				verb = "contains"
			}
			w.Addf("Trailing bytes", "%d %s after the CMS at offset %d %s non-zero data", t.Count, plural(t.Count, "byte"), t.Offset, verb)
		}
		if len(s.Certificates) > 0 {
			w.Addf("Certificates", "%d embedded", len(s.Certificates))
			for j, c := range s.Certificates {
				w.Add(fmt.Sprintf("  cert %d", j+1),
					asciiSafe(c.Subject+" (issuer "+c.Issuer+", serial "+c.Serial+")"))
				addCertExtensionRows(w, c)
			}
		}
		if s.DecomposeError != "" {
			w.Add("Decompose error", s.DecomposeError)
		}
		switch {
		case s.CoverageError != "":
			w.Add("Coverage error", s.CoverageError)
		case len(s.ByteRange) == 4:
			w.Addf("ByteRange", "%v", s.ByteRange)
			if s.CoversWholeFile {
				w.Add("Coverage", "covers the whole file except the /Contents hole")
			} else {
				end := s.ByteRange[2] + s.ByteRange[3]
				size := end + s.TrailingGap
				// Coverage FACT, not a breakage claim: an earlier-revision
				// signature legitimately stops short of EOF.
				w.Addf("Coverage", "signature covers bytes %d..%d of a %d-byte file", s.ByteRange[0], end, size)
				if s.TrailingGap > 0 {
					w.Addf("Coverage note", "later revisions (trailing %d bytes) not covered", s.TrailingGap)
				}
				if s.ByteRange[0] != 0 {
					w.Addf("Coverage note", "signed range does not start at byte 0 (starts at %d)", s.ByteRange[0])
				}
			}
			switch {
			case s.HoleCheckError != "":
				w.Add("Hole matches /Contents", "not checked: "+s.HoleCheckError)
			case s.HoleMatchesContents:
				w.Add("Hole matches /Contents", "yes")
			default:
				w.Add("Hole matches /Contents", "no (excluded span differs from the /Contents extent)")
			}
		}
		for _, n := range s.Notes {
			w.Add("Note", n)
		}
	}
	return w.Render(out)
}

// addCertExtensionRows adds the key usage, extended key usage, CRL DP, OCSP
// and CA Issuers rows of one chain certificate. A nil slice prints "absent";
// an empty one prints "present, no bits set", "present, no OID" or
// "present, no URI".
func addCertExtensionRows(w *kvWriter, c pdfcore.CertInfo) {
	ku := "absent"
	if c.KeyUsage != nil {
		ku = "present, no bits set"
		if len(c.KeyUsage) > 0 {
			ku = strings.Join(c.KeyUsage, ", ")
		}
	}
	w.Add("    Key usage", ku)

	eku := "absent"
	if c.ExtKeyUsage != nil {
		eku = "present, no OID"
		if len(c.ExtKeyUsage) > 0 {
			parts := make([]string, len(c.ExtKeyUsage))
			for i, e := range c.ExtKeyUsage {
				parts[i] = e.OID
				if e.Name != "" {
					parts[i] = e.Name + " (" + e.OID + ")"
				}
			}
			eku = strings.Join(parts, ", ")
		}
	}
	w.Add("    Ext key usage", eku)

	w.Add("    CRL DP", certURLList(c.CRLDistributionPoints))
	w.Add("    OCSP", certURLList(c.OCSPServers))
	w.Add("    CA Issuers", certURLList(c.CAIssuers))
}

// certURLList renders a certificate URL list: "absent" for nil, "present, no
// URI" for empty, otherwise the URLs joined with ", ", each escaped to ASCII.
func certURLList(urls []string) string {
	if urls == nil {
		return "absent"
	}
	if len(urls) == 0 {
		return "present, no URI"
	}
	parts := make([]string, len(urls))
	for i, u := range urls {
		parts[i] = asciiSafe(u)
	}
	return strings.Join(parts, ", ")
}

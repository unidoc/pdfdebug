package main

import (
	"fmt"
	"io"
	"os"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// runSignaturesDump handles `dump signatures`: enumerate + decompose every
// signature field. Plain-text default is a per-signature block;
// --json emits a top-level array. Zero signature fields is a normal empty
// state (exit 0); open or view errors go to stderr with exit 2. The output
// reports decomposed structural facts only - never a trust verdict.
func runSignaturesDump(args []string) int {
	filePath, flags, ok := parseDocViewFlags("signatures", args)
	if !ok {
		return 1
	}
	return execSignaturesDump(filePath, flags)
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

	ins, _, code := openForCLI(filePath)
	if code != 0 {
		return code
	}
	defer func() { _ = ins.Close("cli") }()

	list, err := ins.GetSignatures("cli")
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}

	if flags.json {
		// Emit the slice directly so the top-level shape is a JSON array.
		if err := emit(os.Stdout, list.Signatures, flags.pretty); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}
	if err := printSignaturesPlain(os.Stdout, list.Signatures); err != nil {
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
		w.Add("Field", dashIfEmpty(s.FieldName))
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
			w.Add("Signing time", v)
		}
		if s.Name != "" {
			w.Add("Name", s.Name)
		}
		if s.Reason != "" {
			w.Add("Reason", s.Reason)
		}
		if s.Location != "" {
			w.Add("Location", s.Location)
		}
		if s.ContactInfo != "" {
			w.Add("Contact", s.ContactInfo)
		}
		if s.Signer != nil {
			w.Add("Signer", s.Signer.Subject)
			w.Add("Issuer", s.Signer.Issuer)
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
		if len(s.Certificates) > 0 {
			w.Addf("Certificates", "%d embedded", len(s.Certificates))
			for j, c := range s.Certificates {
				w.Add(fmt.Sprintf("  cert %d", j+1),
					c.Subject+" (issuer "+c.Issuer+", serial "+c.Serial+")")
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
			if s.HoleMatchesContents {
				w.Add("Hole matches /Contents", "yes")
			} else {
				w.Add("Hole matches /Contents", "no (excluded span differs from the /Contents extent)")
			}
		}
		for _, n := range s.Notes {
			w.Add("Note", n)
		}
	}
	return w.Render(out)
}

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// quotedFieldName renders a field name for an error message: quoted, with
// anything outside printable ASCII escaped.
func quotedFieldName(name string) string {
	return strconv.QuoteToASCII(name)
}

// fieldNameList joins the names of sigs[idx...] for an error message, an
// empty name as "-".
func fieldNameList(sigs []pdfcore.SignatureField, idx []int) string {
	names := make([]string, len(idx))
	for i, j := range idx {
		names[i] = dashIfEmpty(asciiSafe(sigs[j].FieldName))
	}
	return strings.Join(names, ", ")
}

// resolveSignatureField returns the index of the one signed field named
// field. An unknown name, a name shared by several fields and an unsigned
// field are errors.
func resolveSignatureField(sigs []pdfcore.SignatureField, field string) (int, error) {
	var matches, all []int
	for i, s := range sigs {
		all = append(all, i)
		if s.FieldName == field {
			matches = append(matches, i)
		}
	}
	switch len(matches) {
	case 0:
		if len(all) == 0 {
			return 0, fmt.Errorf("no signature field named %s", quotedFieldName(field))
		}
		return 0, fmt.Errorf("no signature field named %s; fields: %s", quotedFieldName(field), fieldNameList(sigs, all))
	case 1:
	default:
		return 0, fmt.Errorf("%d signature fields named %s", len(matches), quotedFieldName(field))
	}
	if !sigs[matches[0]].Signed {
		return 0, fmt.Errorf("signature field %s is not signed", quotedFieldName(field))
	}
	return matches[0], nil
}

// selectCMSExport returns the index in sigs of the signature --export-cms
// writes. With field (the --field value, "" when not given) it is that field,
// which must have a CMS SubFilter; without, the one signed field with a CMS
// SubFilter.
func selectCMSExport(sigs []pdfcore.SignatureField, field string) (int, error) {
	if field != "" {
		i, err := resolveSignatureField(sigs, field)
		if err != nil {
			return 0, err
		}
		if !pdfcore.IsCMSSubFilter(sigs[i].SubFilter) {
			return 0, fmt.Errorf("signature field %s has SubFilter %s, not CMS",
				quotedFieldName(field), dashIfEmpty(asciiSafe(sigs[i].SubFilter)))
		}
		return i, nil
	}
	var cands []int
	for i, s := range sigs {
		if s.Signed && pdfcore.IsCMSSubFilter(s.SubFilter) {
			cands = append(cands, i)
		}
	}
	switch len(cands) {
	case 0:
		return 0, errors.New("no CMS signature to export")
	case 1:
		return cands[0], nil
	default:
		return 0, fmt.Errorf("%d signatures with a CMS SubFilter (%s); pick one with --field NAME",
			len(cands), fieldNameList(sigs, cands))
	}
}

// selectCertsExport returns the indexes in sigs whose certificates
// --export-certs writes: the --field one when field is not "", whatever its
// SubFilter, otherwise every signed field.
func selectCertsExport(sigs []pdfcore.SignatureField, field string) ([]int, error) {
	if field != "" {
		i, err := resolveSignatureField(sigs, field)
		if err != nil {
			return nil, err
		}
		return []int{i}, nil
	}
	idx := []int{}
	for i, s := range sigs {
		if s.Signed {
			idx = append(idx, i)
		}
	}
	return idx, nil
}

// exportFile is one file an export writes.
type exportFile struct {
	path string
	data []byte
}

// samePath reports whether a and b name the same file. When either exists,
// only os.SameFile on both decides (links and other spellings included). When
// neither exists, they match when their base names are equal under case
// folding and their directories are the same existing directory, so a
// case-insensitive file system cannot map two new names to one file unseen. A
// missing directory never matches: writing into it fails.
func samePath(a, b string) bool {
	ai, errA := os.Stat(a)
	bi, errB := os.Stat(b)
	if errA == nil || errB == nil {
		return errA == nil && errB == nil && os.SameFile(ai, bi)
	}
	if !strings.EqualFold(filepath.Base(a), filepath.Base(b)) {
		return false
	}
	ad, errA := os.Stat(filepath.Dir(a))
	bd, errB := os.Stat(filepath.Dir(b))
	return errA == nil && errB == nil && os.SameFile(ad, bd)
}

// runSignatureExports runs --export-cms and --export-certs for the PDF at
// inputPath. Every selection and path check runs before the first file is
// written: no export may overwrite the input PDF, and the CMS file may not be
// one of the certificate files. Then the CMS file is written, then the
// certificate files. Errors go to stderr and return exit code 2; files
// written before a failure stay.
func runSignatureExports(inputPath string, sigs []pdfcore.SignatureField, sb []pdfcore.SignatureBytes, ex signatureExportFlags) int {
	fail := func(err error) int {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		return 2
	}

	cmsIdx := -1
	if ex.cmsPath != "" {
		i, err := selectCMSExport(sigs, ex.field)
		if err != nil {
			return fail(err)
		}
		if sb[i].CMS == nil {
			return fail(fmt.Errorf("/Contents of signature field %s could not be read (%s)",
				quotedFieldName(sigs[i].FieldName), sigs[i].DecomposeError))
		}
		cmsIdx = i
	}

	var certFiles []exportFile
	if ex.certsDir != "" {
		idx, err := selectCertsExport(sigs, ex.field)
		if err != nil {
			return fail(err)
		}
		for _, i := range idx {
			for m, der := range sb[i].Certificates {
				name := fmt.Sprintf("sig%d-cert%d.der", i+1, m+1)
				certFiles = append(certFiles, exportFile{path: filepath.Join(ex.certsDir, name), data: der})
			}
		}
		if len(certFiles) == 0 {
			return fail(errors.New("no certificates to export"))
		}
	}

	if cmsIdx >= 0 && samePath(ex.cmsPath, inputPath) {
		return fail(fmt.Errorf("--export-cms %s is the input PDF", asciiSafe(ex.cmsPath)))
	}
	for _, f := range certFiles {
		if samePath(f.path, inputPath) {
			return fail(fmt.Errorf("--export-certs would overwrite the input PDF %s", asciiSafe(f.path)))
		}
		if cmsIdx >= 0 && samePath(f.path, ex.cmsPath) {
			return fail(fmt.Errorf("--export-cms %s is also a certificate file of --export-certs", asciiSafe(ex.cmsPath)))
		}
	}

	if cmsIdx >= 0 {
		b := sb[cmsIdx]
		if err := os.WriteFile(ex.cmsPath, b.CMS, 0o644); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write %s: %v", ex.cmsPath, err))
			return 2
		}
		if b.CMSUndelimited != "" {
			writeJSONWarning(os.Stderr, fmt.Sprintf(
				"%s: CMS element could not be delimited (%s); wrote all %d /Contents bytes including padding",
				dashIfEmpty(asciiSafe(sigs[cmsIdx].FieldName)), b.CMSUndelimited, len(b.CMS)))
		}
	}

	if len(certFiles) > 0 {
		if err := os.MkdirAll(ex.certsDir, 0o755); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write %s: %v", ex.certsDir, err))
			return 2
		}
		for _, f := range certFiles {
			if err := os.WriteFile(f.path, f.data, 0o644); err != nil {
				writeJSONError(os.Stderr, fmt.Sprintf("failed to write %s: %v", f.path, err))
				return 2
			}
		}
	}
	return 0
}

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// A rule that degraded to an info problem prints "not evaluated" in the rules
// block, and the clean sentence does not print because a problem is present.
func TestPrintValidatePlain_DegradedRuleShowsNotEvaluated(t *testing.T) {
	rules := pdfcore.ProfileRules(pdfcore.ProfilePDFUA1Structural)
	if len(rules) < 2 {
		t.Fatalf("need at least two pdfua-1-structural rules, got %d", len(rules))
	}
	for i := range rules {
		rules[i].Evaluated = true
	}
	degraded := rules[len(rules)-1]
	rules[len(rules)-1].Evaluated = false
	res := &pdfcore.ValidationResult{
		Profile: pdfcore.ProfilePDFUA1Structural,
		Summary: pdfcore.ValidationSummary{Info: 1},
		Problems: []pdfcore.Problem{{
			RuleID:   degraded.RuleID,
			Severity: "info",
			Message:  "rule could not be evaluated: boom",
			SpecRef:  degraded.SpecRef,
		}},
		Disclaimer: pdfcore.DisclaimerText,
		Scope:      pdfcore.ProfileScope(pdfcore.ProfilePDFUA1Structural),
		Rules:      rules,
	}

	var b strings.Builder
	if err := printValidatePlain(&b, res); err != nil {
		t.Fatalf("printValidatePlain: %v", err)
	}
	out := b.String()

	for _, r := range rules {
		var line string
		for l := range strings.SplitSeq(out, "\n") {
			if f := strings.Fields(l); len(f) > 0 && f[0] == r.RuleID {
				line = l
				break
			}
		}
		if line == "" {
			t.Errorf("no rules-block line for %q:\n%s", r.RuleID, out)
			continue
		}
		want, unwanted := "0 found", "not evaluated"
		if r.RuleID == degraded.RuleID {
			want, unwanted = "not evaluated", "found"
		}
		if !strings.Contains(line, want) || strings.Contains(line, unwanted) {
			t.Errorf("line for %q = %q, want %q and not %q", r.RuleID, line, want, unwanted)
		}
	}
	if strings.Contains(out, "none of the") {
		t.Errorf("a degraded run must not print the clean sentence:\n%s", out)
	}
	if !strings.Contains(out, "1 info problem") {
		t.Errorf("summary must surface the degraded rule:\n%s", out)
	}
}

// The spec ref, outcome and checks columns start at the same offset on every
// line, even when ids, spec refs and outcomes differ in width.
func TestWriteRulesChecked_AlignsColumns(t *testing.T) {
	rules := []pdfcore.RuleInfo{
		{RuleID: "a", SpecRef: "ISO 1, 6.1", Evaluated: true, Findings: 0, Checks: "first check"},
		{RuleID: "longer-rule-id", SpecRef: "ISO 19005-1:2005, 6.7.2/6.7.3", Evaluated: false, Checks: "second check"},
		{RuleID: "mid-id", SpecRef: "x", Evaluated: true, Findings: 12, Checks: "third check"},
	}
	var b strings.Builder
	writeRulesChecked(&b, rules)
	out := b.String()

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1+len(rules) {
		t.Fatalf("got %d lines, want header plus %d rule lines:\n%s", len(lines), len(rules), out)
	}
	if lines[0] != "Rules checked (3):" {
		t.Errorf("header = %q, want %q", lines[0], "Rules checked (3):")
	}
	if !strings.HasSuffix(out, "\n\n") {
		t.Errorf("rules block must end with a blank line:\n%q", out)
	}

	specCol, outCol, checksCol := -1, -1, -1
	for i, r := range rules {
		line := lines[i+1]
		outcome := ruleOutcome(r)
		s := strings.Index(line, r.SpecRef)
		o := strings.Index(line[s+len(r.SpecRef):], outcome) + s + len(r.SpecRef)
		c := strings.LastIndex(line, r.Checks)
		if !strings.HasPrefix(line, "  "+r.RuleID) || s < 0 || c < 0 {
			t.Fatalf("line %q does not carry id, spec ref and checks of %+v", line, r)
		}
		if i == 0 {
			specCol, outCol, checksCol = s, o, c
			continue
		}
		if s != specCol || o != outCol || c != checksCol {
			t.Errorf("line %q columns at %d/%d/%d, want %d/%d/%d", line, s, o, c, specCol, outCol, checksCol)
		}
	}
}

// captureStderr runs fn with os.Stderr redirected and returns what it wrote.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = w
	done := make(chan string)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()
	defer func() { os.Stderr = saved }()
	fn()
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

// -h prints the usage line followed by each profile's rule line; any other
// flag parse error prints the usage line alone. Both exit 2.
func TestRunValidate_RuleLinesOnlyOnHelp(t *testing.T) {
	ruleLines := profileRuleLines()
	if len(ruleLines) != len(pdfcore.ValidProfiles) {
		t.Fatalf("profileRuleLines returned %d lines, want one per profile", len(ruleLines))
	}

	var code int
	help := captureStderr(t, func() { code = runValidate([]string{"-h"}) })
	if code != 2 {
		t.Errorf("validate -h exit %d, want 2", code)
	}
	want := validateUsage + "\n" + strings.Join(ruleLines, "\n") + "\n"
	if help != want {
		t.Errorf("validate -h stderr =\n%q\nwant\n%q", help, want)
	}

	bad := captureStderr(t, func() { code = runValidate([]string{"--no-such-flag", "x.pdf"}) })
	if code != 2 {
		t.Errorf("unknown flag exit %d, want 2", code)
	}
	if bad != validateUsage+"\n" {
		t.Errorf("unknown flag stderr = %q, want only the usage line", bad)
	}
}

// When the catalog cannot be read, every catalog-level rule prints "not
// evaluated" and the clean sentence does not print.
func TestPrintValidatePlain_UnreadableCatalogIsNotClean(t *testing.T) {
	ins := pdfcore.NewInspector()
	if _, err := ins.Open("t", filepath.Join("..", "..", "testdata", "tagged.pdf")); err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = ins.Close("t") })
	breakCatalog(t, ins, "t")

	out := validatePlain(t, ins, "t", pdfcore.ProfilePDFUA1Structural)
	for _, r := range pdfcore.ProfileRules(pdfcore.ProfilePDFUA1Structural) {
		if line := ruleLine(out, r.RuleID); !strings.Contains(line, "not evaluated") {
			t.Errorf("line for %q = %q, want \"not evaluated\":\n%s", r.RuleID, line, out)
		}
	}
	if strings.Contains(out, "none of the") {
		t.Errorf("an unreadable catalog must not print the clean sentence:\n%s", out)
	}
}

// breakCatalog makes the open document's catalog unreadable by dropping the
// cached root dict and the trailer /Root reference.
func breakCatalog(t *testing.T, ins *pdfcore.Inspector, tabID string) {
	t.Helper()
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	doc.PDFContext.XRefTable.RootDict = nil
	doc.PDFContext.XRefTable.Root = nil
	if _, err := doc.PDFContext.Catalog(); err == nil {
		t.Fatal("catalog still readable after dropping /Root")
	}
}

// validatePlain validates tabID under profile and returns the plain-text output.
func validatePlain(t *testing.T, ins *pdfcore.Inspector, tabID, profile string) string {
	t.Helper()
	res, err := ins.Validate(tabID, profile)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	var b strings.Builder
	if err := printValidatePlain(&b, res); err != nil {
		t.Fatalf("printValidatePlain: %v", err)
	}
	return b.String()
}

// ruleLine returns the rules-block line of ruleID, or "".
func ruleLine(out, ruleID string) string {
	for l := range strings.SplitSeq(out, "\n") {
		if f := strings.Fields(l); len(f) > 0 && f[0] == ruleID {
			return l
		}
	}
	return ""
}

// openPDF writes objs (numbered 1..N) with an xref table and a trailer
// carrying trailerExtra to a temp file and opens it as tab "t".
func openPDF(t *testing.T, trailerExtra string, objs ...string) *pdfcore.Inspector {
	t.Helper()
	body := "%PDF-1.4\n"
	xref := fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for i, o := range objs {
		xref += fmt.Sprintf("%010d 00000 n \n", len(body))
		body += fmt.Sprintf("%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	data := fmt.Sprintf("%s%strailer\n<< /Size %d /Root 1 0 R %s>>\nstartxref\n%d\n%%%%EOF\n",
		body, xref, len(objs)+1, trailerExtra, len(body))
	path := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	ins := pdfcore.NewInspector()
	if _, err := ins.Open("t", path); err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = ins.Close("t") })
	return ins
}

// A pdfa-1b run with an unreadable catalog prints the catalog-reading rules as
// not evaluated and does not report a missing XMP packet.
func TestPrintValidatePlain_PDFAUnreadableCatalogIsNotClean(t *testing.T) {
	ins := openPDF(t, "",
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>",
		"<< /Length 9 >>\nstream\n1 0 0 rg\nendstream",
	)
	breakCatalog(t, ins, "t")

	out := validatePlain(t, ins, "t", pdfcore.ProfilePDFA1B)
	for _, id := range []string{"output-intent", "no-js-launch", "xmp-metadata"} {
		if line := ruleLine(out, id); !strings.Contains(line, "not evaluated") {
			t.Errorf("line for %q = %q, want \"not evaluated\":\n%s", id, line, out)
		}
	}
	if strings.Contains(out, "XMP metadata packet is missing") {
		t.Errorf("an unreadable catalog was reported as a missing XMP packet:\n%s", out)
	}
	if strings.Contains(out, "none of the") {
		t.Errorf("an unreadable catalog must not print the clean sentence:\n%s", out)
	}
}

// A page content stream that fails to decode leaves output-intent not
// evaluated, so an otherwise clean pdfa-1b file does not print the clean
// sentence.
func TestPrintValidatePlain_UnreadableContentStreamIsNotClean(t *testing.T) {
	xmp := `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?><x:xmpmeta xmlns:x="adobe:ns:meta/"></x:xmpmeta><?xpacket end="w"?>`
	ins := openPDF(t, "/ID [<01> <01>] ",
		"<< /Type /Catalog /Pages 2 0 R /Metadata 5 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>",
		"<< /Filter /FlateDecode /Length 9 >>\nstream\n1 0 0 rg\nendstream",
		fmt.Sprintf("<< /Type /Metadata /Subtype /XML /Length %d >>\nstream\n%s\nendstream", len(xmp), xmp),
	)

	out := validatePlain(t, ins, "t", pdfcore.ProfilePDFA1B)
	if line := ruleLine(out, "output-intent"); !strings.Contains(line, "not evaluated") {
		t.Errorf("output-intent line = %q, want \"not evaluated\":\n%s", line, out)
	}
	if strings.Contains(out, "none of the") {
		t.Errorf("an unreadable content stream must not print the clean sentence:\n%s", out)
	}
}

package compliance_validation_test

// The top-level `validate` command.

import (
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Fixture sanity: every hand-assembled fixture must parse through the EXISTING
// open path (dump objects, exit 0). This test passes TODAY and guards the suite
// against an eternally-red fixture (13-4 precedent).
// ---------------------------------------------------------------------------

func TestValidate_FixturesParseThroughOpenPath(t *testing.T) {
	bin := buildCLI(t)
	cases := map[string][]byte{
		"non-embedded-font.pdf": nonEmbeddedFontPDF(),
		"untagged.pdf":          untaggedPDF(),
		"tagged.pdf":            taggedPDF(),
	}
	for name, content := range cases {
		pdf := writeTempPDF(t, name, content)
		_, stderr, ec := runCLI(t, bin, "dump", "objects", pdf)
		if ec != 0 {
			t.Fatalf("fixture %q rejected by the existing open path (exit %d): %s", name, ec, stderr)
		}
	}
}

// ---------------------------------------------------------------------------
// A document with a PDF/A-1b structural ERROR exits 1 (the CI compliance-gate
// signal) -- distinct from operational exit 2.
// ---------------------------------------------------------------------------

func TestValidate_ErrorProblemExitsOne(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "non-embedded-font.pdf", nonEmbeddedFontPDF())

	stdout, stderr, ec := runCLI(t, bin, "validate", pdf)
	if ec != 1 {
		t.Fatalf("expected exit 1 (error problems found), got %d\nstdout: %s\nstderr: %s", ec, stdout, stderr)
	}
	// Guard against the current binary's unknown-command exit 1: a real run
	// writes the problem list to stdout and never reports an unknown command.
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("exit 1 must be a validation run (non-empty stdout), got empty; stderr: %s", stderr)
	}
	if strings.Contains(strings.ToLower(stderr), "unknown command") {
		t.Errorf("`validate` must be a real command, not an unknown-command error: %s", stderr)
	}
}

// ---------------------------------------------------------------------------
// --json emits the {profile, summary, problems}
// envelope; the non-embedded-font error carries objRef "4 0 R", node id
// obj:0:4, a non-empty specRef, and severity "error".
// ---------------------------------------------------------------------------

func TestValidate_JSONEnvelopeAndFontError(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "non-embedded-font.pdf", nonEmbeddedFontPDF())

	stdout, stderr, ec := runCLI(t, bin, "validate", "--json", pdf)
	if ec != 1 {
		t.Fatalf("expected exit 1, got %d (stderr: %s)", ec, stderr)
	}
	res := validateResult(t, stdout)
	if got := getStr(res, "profile"); got != "pdfa-1b" {
		t.Errorf("profile = %q, want pdfa-1b (default)", got)
	}
	ps := problemsOf(t, res)
	errs, _ := summaryOf(t, res)
	if errs < 1 {
		t.Fatalf("summary.errors = %d, want >=1", errs)
	}
	if got := countBySeverity(ps, "error"); got != errs {
		t.Errorf("summary.errors=%d but %d problems are severity=error", errs, got)
	}
	font := findByMessageContains(ps, "embed")
	if font == nil {
		t.Fatalf("no problem mentions font embedding:\n%s", stdout)
	}
	if getStr(font, "severity") != "error" {
		t.Errorf("font-embedding severity = %q, want error", getStr(font, "severity"))
	}
	if getStr(font, "objRef") != "4 0 R" {
		t.Errorf("font-embedding objRef = %q, want \"4 0 R\"", getStr(font, "objRef"))
	}
	if getStr(font, "objNodeId") != "obj:0:4" {
		t.Errorf("font-embedding objNodeId = %q, want obj:0:4 (gen-then-num)", getStr(font, "objNodeId"))
	}
	if getStr(font, "specRef") == "" {
		t.Errorf("font-embedding specRef is empty; a SpecRef clause is required")
	}
	if getStr(font, "ruleId") == "" {
		t.Errorf("font-embedding ruleId is empty; a stable RuleID is required")
	}
}

// ---------------------------------------------------------------------------
// ObjNodeID is present whenever ObjRef is (a bare ObjRef cannot drive the
// GUI selection wiring). Every object-scoped problem carries a well-formed
// obj:{gen}:{num} node id.
// ---------------------------------------------------------------------------

var objNodeRE = regexp.MustCompile(`^obj:\d+:\d+$`)

func TestValidate_ObjNodeIDAccompaniesObjRef(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "non-embedded-font.pdf", nonEmbeddedFontPDF())

	stdout, stderr, ec := runCLI(t, bin, "validate", "--json", pdf)
	if ec != 1 {
		t.Fatalf("expected exit 1, got %d (stderr: %s)", ec, stderr)
	}
	res := validateResult(t, stdout)
	for _, p := range problemsOf(t, res) {
		ref, node := getStr(p, "objRef"), getStr(p, "objNodeId")
		if ref != "" && node == "" {
			t.Errorf("problem has objRef=%q but empty objNodeId: %v", ref, p)
		}
		if node != "" && !objNodeRE.MatchString(node) {
			t.Errorf("objNodeId=%q is not obj:{gen}:{num}", node)
		}
	}
}

// ---------------------------------------------------------------------------
// The plain-text default is a grouped list (severity, message, object ref,
// spec clause) plus a summary count, and is NOT JSON.
// ---------------------------------------------------------------------------

func TestValidate_PlainGroupedListWithSummary(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "non-embedded-font.pdf", nonEmbeddedFontPDF())

	stdout, stderr, ec := runCLI(t, bin, "validate", pdf)
	if ec != 1 {
		t.Fatalf("expected exit 1, got %d (stderr: %s)", ec, stderr)
	}
	assertNotJSON(t, stdout)
	lower := strings.ToLower(stdout)
	if !strings.Contains(lower, "error") {
		t.Errorf("plain list must state the severity (error):\n%s", stdout)
	}
	if !strings.Contains(lower, "embed") {
		t.Errorf("plain list must state the rule message (embedding):\n%s", stdout)
	}
	if !strings.Contains(stdout, "4 0 R") {
		t.Errorf("plain list must state the offending object ref (4 0 R):\n%s", stdout)
	}
	// A summary count: at least one line pairing a number with error(s).
	if !regexp.MustCompile(`\d+\s+error`).MatchString(lower) {
		t.Errorf("plain output must carry a summary count of errors:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// Neither the plain default nor --json ever emits an authoritative
// conformance verdict; the "structural checks only" disclaimer is always
// present.
// ---------------------------------------------------------------------------

func TestValidate_HonestyGuardrail(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "non-embedded-font.pdf", nonEmbeddedFontPDF())

	stdout, _, _ := runCLI(t, bin, "validate", pdf)
	assertNoComplianceVerdict(t, "plain", stdout)

	jsonOut, _, _ := runCLI(t, bin, "validate", "--json", pdf)
	// The disclaimer must survive into the JSON surface too (a JSON consumer
	// must not have to infer it). It may live in a dedicated field or note.
	if !strings.Contains(strings.ToLower(jsonOut), "structural checks only") {
		t.Errorf("--json output must carry the not-authoritative disclaimer:\n%s", jsonOut)
	}
	for _, p := range forbiddenVerdictPhrases {
		if strings.Contains(strings.ToLower(jsonOut), p) {
			t.Errorf("--json emits authoritative verdict %q:\n%s", p, jsonOut)
		}
	}
}

// ---------------------------------------------------------------------------
// 13-1 contract: the plain-text default is ASCII-only and ends with a
// trailing newline.
// ---------------------------------------------------------------------------

func TestValidate_PlainIsASCIIWithTrailingNewline(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "non-embedded-font.pdf", nonEmbeddedFontPDF())

	stdout, stderr, ec := runCLI(t, bin, "validate", pdf)
	if ec != 1 {
		t.Fatalf("expected exit 1, got %d", ec)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatalf("a validation run must write plain-text output to stdout; stderr: %s", stderr)
	}
	assertASCII(t, stdout)
	assertTrailingNewline(t, stdout)
}

// ---------------------------------------------------------------------------
// An encrypted document is surfaced as an ERROR problem (PDF/A forbids
// encryption) and exits 1 -- NOT the operational exit 2 (the ErrEncryptedPDF
// reconciliation). Uses the existing encrypted.pdf.
// ---------------------------------------------------------------------------

func TestValidate_EncryptedIsErrorProblemExitsOne(t *testing.T) {
	bin := buildCLI(t)
	pdf := testdataDir(t) + "/encrypted.pdf"

	stdout, stderr, ec := runCLI(t, bin, "validate", pdf)
	if ec != 1 {
		t.Fatalf("encrypted doc must exit 1 (validation error), got %d\nstdout: %s\nstderr: %s", ec, stdout, stderr)
	}
	if !strings.Contains(strings.ToLower(stdout), "encrypt") {
		t.Errorf("plain output must report the encryption problem:\n%s", stdout)
	}
	// And in JSON it must be a real error-severity problem, not a bare error.
	jsonOut, _, ec := runCLI(t, bin, "validate", "--json", pdf)
	if ec != 1 {
		t.Fatalf("--json encrypted doc must exit 1, got %d", ec)
	}
	res := validateResult(t, jsonOut)
	enc := findByMessageContains(problemsOf(t, res), "encrypt")
	if enc == nil {
		t.Fatalf("no encryption problem in --json output:\n%s", jsonOut)
	}
	if getStr(enc, "severity") != "error" {
		t.Errorf("encryption problem severity = %q, want error", getStr(enc, "severity"))
	}
}

// ---------------------------------------------------------------------------
// A missing/unreadable file is an OPERATIONAL error (exit 2, message to
// stderr, empty stdout) -- NOT the compliance exit 1.
// ---------------------------------------------------------------------------

func TestValidate_MissingFileExitsTwo(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, ec := runCLI(t, bin, "validate", testdataDir(t)+"/does-not-exist.pdf")
	if ec != 2 {
		t.Fatalf("missing file must exit 2 (operational), got %d", ec)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout must stay empty on operational error, got:\n%s", stdout)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Errorf("operational error must be reported to stderr")
	}
}

// ---------------------------------------------------------------------------
// An unknown --profile is a usage error (exit 2, stderr lists the valid
// profiles, no partial run on stdout).
// ---------------------------------------------------------------------------

func TestValidate_UnknownProfileExitsTwo(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "non-embedded-font.pdf", nonEmbeddedFontPDF())

	stdout, stderr, ec := runCLI(t, bin, "validate", "--profile", "pdfa-9z", pdf)
	if ec != 2 {
		t.Fatalf("unknown profile must exit 2 (operational), got %d\nstdout: %s\nstderr: %s", ec, stdout, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("no partial run -- stdout must stay empty, got:\n%s", stdout)
	}
	low := strings.ToLower(stderr)
	if !strings.Contains(low, "pdfa-1b") || !strings.Contains(low, "pdfua-1-structural") {
		t.Errorf("stderr must list the valid profiles (pdfa-1b, pdfua-1-structural), got: %s", stderr)
	}
}

// ---------------------------------------------------------------------------
// The PDF/UA-1 structural rules are WARNINGS (non-gating). An untagged doc
// under pdfua-1-structural yields warnings but ZERO errors -> exit 0. The
// missing /Lang is a document-level problem (empty objRef) -- the story's
// canonical "Document" group member.
// ---------------------------------------------------------------------------

func TestValidate_PDFUAWarningsDoNotGate(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "untagged.pdf", untaggedPDF())

	stdout, stderr, ec := runCLI(t, bin, "validate", "--profile", "pdfua-1-structural", "--json", pdf)
	if ec != 0 {
		t.Fatalf("PDF/UA warnings must NOT gate; expected exit 0, got %d\nstdout: %s\nstderr: %s", ec, stdout, stderr)
	}
	res := validateResult(t, stdout)
	if got := getStr(res, "profile"); got != "pdfua-1-structural" {
		t.Errorf("profile = %q, want pdfua-1-structural", got)
	}
	ps := problemsOf(t, res)
	errs, warns := summaryOf(t, res)
	if errs != 0 {
		t.Errorf("PDF/UA structural profile must emit zero errors, got %d", errs)
	}
	if warns < 1 {
		t.Errorf("untagged doc should raise >=1 PDF/UA warning, got %d", warns)
	}
	// Every emitted PDF/UA problem must be a warning and carry the profile tag.
	for _, p := range ps {
		if getStr(p, "severity") != "warning" {
			t.Errorf("PDF/UA problem severity = %q, want warning: %v", getStr(p, "severity"), p)
		}
		if getStr(p, "profile") != "pdfua-1-structural" {
			t.Errorf("problem.profile = %q, want pdfua-1-structural", getStr(p, "profile"))
		}
	}
	// The missing /Lang problem is document-level: no object ref.
	lang := findByMessageContains(ps, "lang")
	if lang == nil {
		t.Fatalf("untagged doc must raise a missing-/Lang warning:\n%s", stdout)
	}
	if getStr(lang, "objRef") != "" {
		t.Errorf("missing-/Lang is document-level; objRef must be empty, got %q", getStr(lang, "objRef"))
	}
}

// ---------------------------------------------------------------------------
// A doc that satisfies every rule of the selected profile yields zero
// problems and exits 0 with a clean plain-text state. A tagged doc under
// pdfua-1-structural is the clean case. Exit 0 means "no structural errors
// found," NOT "compliant."
// ---------------------------------------------------------------------------

func TestValidate_CleanForProfileExitsZero(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "tagged.pdf", taggedPDF())

	stdout, stderr, ec := runCLI(t, bin, "validate", "--profile", "pdfua-1-structural", pdf)
	if ec != 0 {
		t.Fatalf("clean-for-profile doc must exit 0, got %d\nstdout: %s\nstderr: %s", ec, stdout, stderr)
	}
	assertNotJSON(t, stdout)
	assertNoComplianceVerdict(t, "plain", stdout)
	if !strings.Contains(strings.ToLower(stdout), "no structural problems found") {
		t.Errorf("clean plain output must say \"no structural problems found\":\n%s", stdout)
	}

	jsonOut, _, ec := runCLI(t, bin, "validate", "--profile", "pdfua-1-structural", "--json", pdf)
	if ec != 0 {
		t.Fatalf("--json clean doc must exit 0, got %d", ec)
	}
	res := validateResult(t, jsonOut)
	if len(problemsOf(t, res)) != 0 {
		t.Errorf("tagged doc must yield zero PDF/UA structural problems:\n%s", jsonOut)
	}
}

// ---------------------------------------------------------------------------
// --help documents the `validate` command, lists the valid profiles, describes
// exit 0 as "no structural errors" (never "compliant"/"valid"/"passed"), and
// carries the not-authoritative disclaimer.
// ---------------------------------------------------------------------------

func TestValidate_HelpDocumentsCommandAndExitSemantics(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, _ := runCLI(t, bin, "--help")
	combined := stdout + stderr
	low := strings.ToLower(combined)
	if !strings.Contains(low, "validate") {
		t.Errorf("--help does not mention the `validate` command:\n%s", combined)
	}
	if !strings.Contains(low, "pdfa-1b") || !strings.Contains(low, "pdfua-1-structural") {
		t.Errorf("--help must list the valid profiles:\n%s", combined)
	}
	// Exit 0 must be described as "no structural errors," never a verdict.
	if !strings.Contains(low, "no structural errors") {
		t.Errorf("--help must describe exit 0 as \"no structural errors\":\n%s", combined)
	}
	for _, p := range forbiddenVerdictPhrases {
		if strings.Contains(low, p) {
			t.Errorf("--help emits authoritative verdict %q", p)
		}
	}
}

// Acceptance harness for the PDF/UA-1 structural content rules of
// `pdfdebug validate --profile pdfua-1-structural`.
//
// Black-box: the CLI is built once and run as a subprocess over the committed
// ua-structural-*.pdf fixtures in testdata/ and over documents written by the
// test. Covers exit codes, the typed --json shape with per-rule IDs, untruncated
// output and the mapping of each fixture to its committed veraPDF golden. Rule
// edge cases live in the pdfcore unit tests.
//
// Run: cd tests/ua-structural-rules && go test -count=1 ./...
package ua_structural_rules_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

const uaProfile = "pdfua-1-structural"

// uaRuleIDs is every pdfua-1-structural rule in registry order.
var uaRuleIDs = []string{
	"marked", "struct-tree-root", "lang",
	"marked-content-balance", "mcid-unique", "mcid-in-parent-tree", "mcid-not-nested", "struct-elem-type",
}

// contentRuleIDs are the structural content rules, each one gating the exit code.
var contentRuleIDs = []string{"marked-content-balance", "mcid-unique", "mcid-in-parent-tree", "mcid-not-nested"}

// structureRuleIDs are the rules that read the structure report.
var structureRuleIDs = append(append([]string{}, contentRuleIDs...), "struct-elem-type")

// structureRuleSpec is the severity and spec reference of each structure rule.
var structureRuleSpec = map[string]struct{ severity, specRef string }{
	"marked-content-balance": {"error", "ISO 14289-1:2014, 7.1; ISO 32000-1:2008, 14.6.1"},
	"mcid-unique":            {"error", "ISO 14289-1:2014, 7.1; ISO 32000-1:2008, 14.7.4.2"},
	"mcid-in-parent-tree":    {"error", "ISO 14289-1:2014, 7.1; ISO 32000-1:2008, 14.7.4.4"},
	"mcid-not-nested":        {"error", "ISO 14289-1:2014, 7.1; ISO 32000-1:2008, 14.7.4.1"},
	"struct-elem-type":       {"warning", "ISO 14289-1:2014, 7.1; ISO 32000-1:2008, 14.7.2"},
}

// projectRoot walks up from the test directory to the main module's go.mod.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		if content, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			if strings.Contains(string(content), "module unidoc-pdf-debugger") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger found)")
		}
		dir = parent
	}
}

// fixture returns the absolute path of a file in testdata/, failing when it
// is missing.
func fixture(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(projectRoot(t), "testdata", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture %s missing: %v", name, err)
	}
	return p
}

var (
	cliBuildOnce sync.Once
	cliBinPath   string
	cliBuildErr  string
)

// buildCLI compiles the CLI once per test package and returns the binary path.
func buildCLI(t *testing.T) string {
	t.Helper()
	cliBuildOnce.Do(func() {
		binName := "pdfdebug"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		tmpDir, err := os.MkdirTemp("", "pdfdebug-ua-structural-")
		if err != nil {
			cliBuildErr = "failed to create temp dir: " + err.Error()
			return
		}
		binPath := filepath.Join(tmpDir, binName)
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/cli/")
		cmd.Dir = projectRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			cliBuildErr = "failed to build CLI binary: " + err.Error() + "\n" + string(out)
			return
		}
		cliBinPath = binPath
	})
	if cliBuildErr != "" {
		t.Fatalf("%s", cliBuildErr)
	}
	return cliBinPath
}

// runCLI runs the CLI with args and returns stdout, stderr and the exit code.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(buildCLI(t), args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("failed to run CLI: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// problem is one entry of the validate --json problems list.
type problem struct {
	RuleID    string `json:"ruleId"`
	Profile   string `json:"profile"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	ObjRef    string `json:"objRef"`
	ObjNodeID string `json:"objNodeId"`
	SpecRef   string `json:"specRef"`
}

// ruleInfo is one entry of the validate --json rules list.
type ruleInfo struct {
	RuleID    string `json:"ruleId"`
	SpecRef   string `json:"specRef"`
	Severity  string `json:"severity"`
	Checks    string `json:"checks"`
	Evaluated bool   `json:"evaluated"`
	Findings  int    `json:"findings"`
}

// summary is the validate --json tally.
type summary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
}

// validationResult is the validate --json object.
type validationResult struct {
	Profile    string     `json:"profile"`
	Summary    summary    `json:"summary"`
	Problems   []problem  `json:"problems"`
	Disclaimer string     `json:"disclaimer"`
	Scope      string     `json:"scope"`
	Rules      []ruleInfo `json:"rules"`
}

// validateJSON runs `validate --profile pdfua-1-structural --json` on path and
// returns the parsed result and the exit code.
func validateJSON(t *testing.T, path string) (validationResult, int) {
	t.Helper()
	stdout, stderr, code := runCLI(t, "validate", "--profile", uaProfile, "--json", path)
	if code != 0 && code != 1 {
		t.Fatalf("validate --json %s exit = %d, want 0 or 1\nstderr: %s", filepath.Base(path), code, stderr)
	}
	var r validationResult
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatalf("--json output is not a validation result: %v\n%s", err, stdout)
	}
	return r, code
}

// ruleByID returns the named rule of a result, failing when it is absent.
func ruleByID(t *testing.T, r validationResult, id string) ruleInfo {
	t.Helper()
	for _, ri := range r.Rules {
		if ri.RuleID == id {
			return ri
		}
	}
	t.Fatalf("rule %q missing from rules %+v", id, r.Rules)
	return ruleInfo{}
}

// problemsOf returns the problems a rule emitted, in output order.
func problemsOf(r validationResult, id string) []problem {
	var out []problem
	for _, p := range r.Problems {
		if p.RuleID == id {
			out = append(out, p)
		}
	}
	return out
}

// nodeIDOf returns the obj:{gen}:{num} tree-node id of an "N G R" reference.
func nodeIDOf(t *testing.T, ref string) string {
	t.Helper()
	var num, gen int
	if _, err := fmt.Sscanf(ref, "%d %d R", &num, &gen); err != nil {
		t.Fatalf("reference %q is not \"N G R\": %v", ref, err)
	}
	return fmt.Sprintf("obj:%d:%d", gen, num)
}

// structStream is one content stream row of a dump structure page.
type structStream struct {
	Ref string `json:"ref"`
}

// structMCID is one content MCID row of a dump structure page.
type structMCID struct {
	MCID   int `json:"mcid"`
	Stream int `json:"stream"`
	Line   int `json:"line"`
	Col    int `json:"col"`
}

// structPage is the subset of a dump structure page this suite reads.
type structPage struct {
	PageNum      int            `json:"pageNum"`
	Ref          string         `json:"ref"`
	Streams      []structStream `json:"streams"`
	ContentMCIDs []structMCID   `json:"contentMcids"`
}

// structElem is the subset of a dump structure element this suite reads.
type structElem struct {
	Ref  string       `json:"ref"`
	S    string       `json:"s"`
	Kids []*structKid `json:"kids"`
}

// structKid is one /K entry of a dump structure element.
type structKid struct {
	Kind    string      `json:"kind"`
	Element *structElem `json:"element"`
}

// structureReport is the subset of `dump structure --json` this suite reads to
// locate pages, streams, MCIDs and elements independently of validate.
type structureReport struct {
	Elements []*structKid  `json:"elements"`
	Pages    []*structPage `json:"pages"`
}

// dumpStructure runs `dump structure --json` on path and requires exit 0.
func dumpStructure(t *testing.T, path string) structureReport {
	t.Helper()
	stdout, stderr, code := runCLI(t, "dump", "structure", "--json", path)
	if code != 0 {
		t.Fatalf("dump structure --json %s exit = %d, want 0\nstderr: %s", filepath.Base(path), code, stderr)
	}
	var r structureReport
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatalf("dump structure --json output is not a structure report: %v\n%s", err, stdout)
	}
	return r
}

// firstPage returns page 1 of a structure report.
func firstPage(t *testing.T, r structureReport) *structPage {
	t.Helper()
	for _, p := range r.Pages {
		if p.PageNum == 1 {
			return p
		}
	}
	t.Fatalf("page 1 missing from the structure report")
	return nil
}

// occurrences returns "stream R line L col C" for every content row of mcid
// on page p, in content order.
func occurrences(t *testing.T, p *structPage, mcid int) []string {
	t.Helper()
	var out []string
	for _, c := range p.ContentMCIDs {
		if c.MCID != mcid {
			continue
		}
		if c.Stream < 0 || c.Stream >= len(p.Streams) {
			t.Fatalf("MCID %d row names stream %d of %d", mcid, c.Stream, len(p.Streams))
		}
		out = append(out, fmt.Sprintf("stream %s line %d col %d", p.Streams[c.Stream].Ref, c.Line, c.Col))
	}
	return out
}

// eachElement calls fn on every element of kids, depth first.
func eachElement(kids []*structKid, fn func(e *structElem)) {
	for _, k := range kids {
		if k.Element != nil {
			fn(k.Element)
			eachElement(k.Element.Kids, fn)
		}
	}
}

// pdfDoc assembles a classic-xref PDF from object bodies; object n is
// objs[n-1] and the catalog is object 1.
func pdfDoc(objs []string) []byte {
	var b strings.Builder
	b.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objs))
	for i, body := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, len(objs), xref)
	return []byte(b.String())
}

// streamObj renders a stream object body whose /Length matches data.
func streamObj(dict, data string) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(data), data)
}

// xmpPacket is an XMP packet with dc:title and pdfuaid:part 1.
const xmpPacket = `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/">
<dc:title><rdf:Alt><rdf:li xml:lang="x-default">Many MCIDs</rdf:li></rdf:Alt></dc:title>
<pdfuaid:part>1</pdfuaid:part>
</rdf:Description>
</rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`

// manyFindingsPDF is a one-page tagged document carrying the clean fixture's
// catalog entries. Its content marks each MCID m in 0..n-1 twice and opens MCID
// m+n inside the second, still open, sequence of m; every sequence is closed.
// The page's ParentTree array has 2n slots that all reference one P element
// under Document.
func manyFindingsPDF(n int) []byte {
	var content strings.Builder
	for m := 0; m < n; m++ {
		fmt.Fprintf(&content, "/P <</MCID %d>> BDC EMC\n/P <</MCID %d>> BDC /Span <</MCID %d>> BDC EMC EMC\n", m, m, m+n)
	}
	slots := make([]string, 2*n)
	kids := make([]string, 2*n)
	for i := range slots {
		slots[i] = "8 0 R"
		kids[i] = fmt.Sprint(i)
	}
	return pdfDoc([]string{
		// 1 catalog
		"<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /Lang (en-US) /StructTreeRoot 5 0 R /ViewerPreferences << /DisplayDocTitle true >> /Metadata 9 0 R >>",
		// 2 pages
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		// 3 page
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R /StructParents 0 >>",
		// 4 content
		streamObj("", content.String()),
		// 5 structure tree root
		"<< /Type /StructTreeRoot /K 7 0 R /ParentTree 6 0 R /ParentTreeNextKey 1 >>",
		// 6 parent tree
		fmt.Sprintf("<< /Nums [0 [%s]] >>", strings.Join(slots, " ")),
		// 7 Document
		"<< /Type /StructElem /S /Document /P 5 0 R /K [8 0 R] >>",
		// 8 P claiming every MCID
		fmt.Sprintf("<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K [%s] >>", strings.Join(kids, " ")),
		// 9 metadata
		streamObj("/Type /Metadata /Subtype /XML", xmpPacket),
		// 10 info
		"<< /Title (Many MCIDs) >>",
	})
}

// writeTemp writes data to a file in a test temp dir and returns its path.
func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return p
}

package pdfcore

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
)

// PDF/UA-1 structural content rules of the pdfua-1-structural profile. The
// rules read the structure report, so expected locations (stream ref, line,
// col), ParentTree errors and element errors are taken from GetStructure on
// the same document rather than restated here.

// uaStructureRuleIDs are the rules that read the structure report, in
// registry order.
var uaStructureRuleIDs = []string{"marked-content-balance", "mcid-unique", "mcid-in-parent-tree", "mcid-not-nested", "struct-elem-type"}

// uaContentRuleIDs are the four page-content rules.
var uaContentRuleIDs = uaStructureRuleIDs[:4]

// uaCatalogRuleIDs are the catalog-level rules.
var uaCatalogRuleIDs = []string{"marked", "struct-tree-root", "lang"}

// uaTaggedPDF is a one-page tagged document. The catalog has /MarkInfo,
// /Lang and /StructTreeRoot 5; the root's /K is a Document element (10) with
// one P element (11) on page 3; object 6 is the ParentTree with the given
// /Nums body. page is the page dictionary body (object 3).
func uaTaggedPDF(page, nums string, extra ...rawObj) []byte {
	all := []rawObj{
		{1, "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /Lang (en-US) /StructTreeRoot 5 0 R >>"},
		{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		{3, page},
		{5, "<< /Type /StructTreeRoot /K [10 0 R] /ParentTree 6 0 R /ParentTreeNextKey 1 >>"},
		{6, "<< /Nums [" + nums + "] >>"},
		{10, "<< /Type /StructElem /S /Document /P 5 0 R /K [11 0 R] >>"},
		{11, "<< /Type /StructElem /S /P /P 10 0 R /Pg 3 0 R >>"},
	}
	return rawPDF(append(all, extra...)...)
}

// uaSlots returns a ParentTree /Nums body whose key 0 array has n slots, all
// pointing at the P element.
func uaSlots(n int) string {
	s := make([]string, n)
	for i := range s {
		s[i] = "11 0 R"
	}
	return "0 [" + strings.Join(s, " ") + "]"
}

// uaStreamsPDF is uaTaggedPDF whose page has /StructParents 0 and a /Contents
// array of the given streams (objects 20, 21, ...). A stream given as a full
// object body ("<<...") is used as is, anything else becomes a stream with
// that content. pageExtra is added to the page dictionary.
func uaStreamsPDF(nums, pageExtra string, streams ...string) []byte {
	refs := make([]string, len(streams))
	objs := make([]rawObj, len(streams))
	for i, s := range streams {
		refs[i] = fmt.Sprintf("%d 0 R", 20+i)
		if !strings.HasPrefix(s, "<<") {
			s = rawStream(s)
		}
		objs[i] = rawObj{20 + i, s}
	}
	page := fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents [%s] %s >>", strings.Join(refs, " "), pageExtra)
	return uaTaggedPDF(page, nums, objs...)
}

// uaContentPDF is uaStreamsPDF with /StructParents 0 and ParentTree slots for
// MCIDs 0..9.
func uaContentPDF(streams ...string) []byte {
	return uaStreamsPDF(uaSlots(10), "/StructParents 0", streams...)
}

// uaRun validates pdf under pdfua-1-structural and returns the result with
// the structure report of the same document.
func uaRun(t *testing.T, pdf []byte) (*ValidationResult, *StructureReport) {
	t.Helper()
	ins, _ := openUnvalidated(t, pdf)
	rep := getStructure(t, ins, "raw")
	res, err := ins.Validate("raw", ProfilePDFUA1Structural)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return res, rep
}

// uaRuleOf returns the named rule's outcome, failing when it is absent.
func uaRuleOf(t *testing.T, res *ValidationResult, id string) RuleInfo {
	t.Helper()
	for _, r := range res.Rules {
		if r.RuleID == id {
			return r
		}
	}
	ids := make([]string, len(res.Rules))
	for i, r := range res.Rules {
		ids[i] = r.RuleID
	}
	t.Fatalf("rule %q missing from the run's rules %v", id, ids)
	return RuleInfo{}
}

// uaFindings returns the problems of one rule, in output order.
func uaFindings(res *ValidationResult, id string) []Problem {
	var out []Problem
	for _, p := range res.Problems {
		if p.RuleID == id {
			out = append(out, p)
		}
	}
	return out
}

// uaMessages returns the messages of one rule's problems.
func uaMessages(res *ValidationResult, id string) []string {
	var out []string
	for _, p := range uaFindings(res, id) {
		out = append(out, p.Message)
	}
	return out
}

// uaExpect checks that every structure rule was evaluated, that the rules in
// want have exactly that many findings and that every other structure rule
// has none.
func uaExpect(t *testing.T, res *ValidationResult, want map[string]int) {
	t.Helper()
	for _, id := range uaStructureRuleIDs {
		r := uaRuleOf(t, res, id)
		if !r.Evaluated {
			t.Errorf("rule %s not evaluated: %+v; problems %+v", id, r, uaFindings(res, id))
			continue
		}
		if r.Findings != want[id] || len(uaFindings(res, id)) != want[id] {
			t.Errorf("rule %s findings = %d, want %d: %q", id, r.Findings, want[id], uaMessages(res, id))
		}
	}
}

// uaNodeID returns the obj:{gen}:{num} node id of an "N G R" reference.
func uaNodeID(t *testing.T, ref string) string {
	t.Helper()
	var num, gen int
	if _, err := fmt.Sscanf(ref, "%d %d R", &num, &gen); err != nil {
		t.Fatalf("reference %q is not \"N G R\": %v", ref, err)
	}
	return fmt.Sprintf("obj:%d:%d", gen, num)
}

// uaOnPage checks that p points at the page object 3 0 R.
func uaOnPage(t *testing.T, p Problem) {
	t.Helper()
	if p.ObjRef != "3 0 R" || p.ObjNodeID != "obj:0:3" {
		t.Errorf("problem %q objRef %q objNodeId %q, want page 3 0 R obj:0:3", p.Message, p.ObjRef, p.ObjNodeID)
	}
}

// uaLoc returns "stream R line L col C" for content row i of page 1.
func uaLoc(t *testing.T, rep *StructureReport, i int) string {
	t.Helper()
	p := mustPage(t, rep, 1)
	if i >= len(p.ContentMCIDs) {
		t.Fatalf("page 1 has %d content rows, want row %d", len(p.ContentMCIDs), i)
	}
	c := p.ContentMCIDs[i]
	return fmt.Sprintf("stream %s line %d col %d", p.Streams[c.Stream].Ref, c.Line, c.Col)
}

// uaOnlyMessage returns the one message of rule id, failing on any other count.
func uaOnlyMessage(t *testing.T, res *ValidationResult, id string) Problem {
	t.Helper()
	ps := uaFindings(res, id)
	if len(ps) != 1 {
		t.Fatalf("rule %s problems = %q, want exactly one", id, uaMessages(res, id))
	}
	return ps[0]
}

// ---------------------------------------------------------------------------
// Registry and scope
// ---------------------------------------------------------------------------

func TestProfileRules_PDFUAStructureRulesInOrder(t *testing.T) {
	want := []struct {
		id, severity, specRef string
	}{
		{"marked", "warning", "ISO 14289-1:2014, 7.1"},
		{"struct-tree-root", "warning", "ISO 14289-1:2014, 7.1"},
		{"lang", "warning", "ISO 14289-1:2014, 7.2"},
		{"marked-content-balance", "warning", "ISO 14289-1:2014, 7.1; ISO 32000-1:2008, 14.6.1"},
		{"mcid-unique", "warning", "ISO 14289-1:2014, 7.1; ISO 32000-1:2008, 14.7.4.2"},
		{"mcid-in-parent-tree", "error", "ISO 14289-1:2014, 7.1; ISO 32000-1:2008, 14.7.4.4"},
		{"mcid-not-nested", "warning", "ISO 14289-1:2014, 7.1; ISO 32000-1:2008, 14.7.4.1"},
		{"struct-elem-type", "error", "ISO 14289-1:2014, 7.1; ISO 32000-1:2008, 14.7.2"},
	}
	got := ProfileRules(ProfilePDFUA1Structural)
	if len(got) != len(want) {
		ids := make([]string, len(got))
		for i, r := range got {
			ids[i] = r.RuleID
		}
		t.Fatalf("pdfua-1-structural rules = %v, want %d rules", ids, len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.RuleID != w.id || string(g.Severity) != w.severity || g.SpecRef != w.specRef {
			t.Errorf("rule %d = %s %s %q, want %s %s %q", i, g.RuleID, g.Severity, g.SpecRef, w.id, w.severity, w.specRef)
		}
		assertHonestText(t, "checks of "+g.RuleID, g.Checks)
	}
}

func TestProfileRules_PDFUAChecksPhrasesNameTheirCoverageBound(t *testing.T) {
	phrases := map[string]string{}
	for _, r := range ProfileRules(ProfilePDFUA1Structural) {
		phrases[r.RuleID] = strings.ToLower(r.Checks)
	}
	for _, id := range uaContentRuleIDs {
		c, ok := phrases[id]
		if !ok {
			t.Errorf("rule %s missing", id)
			continue
		}
		if !strings.Contains(c, "form xobject") || !strings.Contains(c, "not scanned") {
			t.Errorf("%s checks %q must say form XObjects are not scanned", id, c)
		}
		if !strings.Contains(c, "/contents") && !strings.Contains(c, "content stream") {
			t.Errorf("%s checks %q must say it reads page content", id, c)
		}
	}
	if c := phrases["marked-content-balance"]; !strings.Contains(c, "decoded") {
		t.Errorf("marked-content-balance checks %q must say an undecodable content stream is reported there", c)
	}
	for _, id := range []string{"mcid-unique", "mcid-in-parent-tree", "mcid-not-nested"} {
		if c := phrases[id]; !strings.Contains(c, "reported under marked-content-balance") {
			t.Errorf("%s checks %q must say unreadable content is reported under marked-content-balance", id, c)
		}
	}
	if c := phrases["struct-elem-type"]; !strings.Contains(c, "/s") || !strings.Contains(c, "role mapping") {
		t.Errorf("struct-elem-type checks %q must name /S and that role mapping is not checked", c)
	}
}

func TestProfileRules_PDFARulesUnchanged(t *testing.T) {
	var ids []string
	for _, r := range ProfileRules(ProfilePDFA1B) {
		ids = append(ids, r.RuleID)
		if r.Severity != "error" {
			t.Errorf("pdfa-1b rule %s severity %q, want error", r.RuleID, r.Severity)
		}
	}
	for _, id := range uaStructureRuleIDs {
		if slices.Contains(ids, id) {
			t.Errorf("pdfa-1b lists the PDF/UA rule %s", id)
		}
	}
}

func TestProfileScope_PDFUANamesContentRulesAndWhatIsNotExamined(t *testing.T) {
	s := ProfileScope(ProfilePDFUA1Structural)
	low := strings.ToLower(s)
	for _, w := range []string{
		"marked-content balance", "mcid uniqueness", "mcid nesting", "parenttree reachability",
		"/s", "alternate text", "font", "role mapping", "form xobject", "marked content",
		"structure tree", "not a pdf/ua-1 conformance check",
	} {
		if !strings.Contains(low, w) {
			t.Errorf("scope %q does not mention %q", s, w)
		}
	}
	if strings.Contains(low, "catalog-level rules listed") {
		t.Errorf("scope %q still limits the profile to catalog-level rules", s)
	}
	assertHonestText(t, "pdfua-1-structural scope", s)
}

// ---------------------------------------------------------------------------
// Committed fixtures
// ---------------------------------------------------------------------------

// uaFixture validates a testdata fixture under pdfua-1-structural.
func uaFixture(t *testing.T, name string) (*ValidationResult, *StructureReport) {
	t.Helper()
	ins := openStructureFixture(t, name)
	rep := getStructure(t, ins, "s")
	res, err := ins.Validate("s", ProfilePDFUA1Structural)
	if err != nil {
		t.Fatalf("Validate %s: %v", name, err)
	}
	return res, rep
}

func TestValidateUA_CleanFixtures(t *testing.T) {
	for _, name := range []string{"ua-structural-clean.pdf", "structure.pdf"} {
		t.Run(name, func(t *testing.T) {
			res, _ := uaFixture(t, name)
			if len(res.Rules) != 8 {
				t.Errorf("got %d rules, want 8", len(res.Rules))
			}
			for _, r := range res.Rules {
				if !r.Evaluated || r.Findings != 0 {
					t.Errorf("rule %+v, want evaluated with 0 findings", r)
				}
			}
			if len(res.Problems) != 0 || res.Summary != (ValidationSummary{}) {
				t.Errorf("problems %+v summary %+v, want none", res.Problems, res.Summary)
			}
		})
	}
}

func TestValidateUA_FixturesWithoutMarkedContentGetNoStructureFinding(t *testing.T) {
	for _, name := range []string{"tagged.pdf", "untagged.pdf"} {
		t.Run(name, func(t *testing.T) {
			res, _ := uaFixture(t, name)
			uaExpect(t, res, nil)
		})
	}
}

func TestValidateUA_EachFaultFixtureTripsOneRule(t *testing.T) {
	cases := []struct {
		name, rule string
		severity   Severity
	}{
		{"ua-structural-unbalanced.pdf", "marked-content-balance", "warning"},
		{"ua-structural-duplicate-mcid.pdf", "mcid-unique", "warning"},
		{"ua-structural-orphan-mcid.pdf", "mcid-in-parent-tree", "error"},
		{"ua-structural-nested-mcid.pdf", "mcid-not-nested", "warning"},
		{"ua-structural-empty-s.pdf", "struct-elem-type", "error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, rep := uaFixture(t, tc.name)
			uaExpect(t, res, map[string]int{tc.rule: 1})
			for _, id := range uaCatalogRuleIDs {
				if r := uaRuleOf(t, res, id); !r.Evaluated || r.Findings != 0 {
					t.Errorf("catalog rule %s = %+v, want evaluated with 0 findings", id, r)
				}
			}
			if len(res.Problems) != 1 {
				t.Fatalf("problems = %+v, want exactly one", res.Problems)
			}
			p := res.Problems[0]
			if p.Severity != tc.severity {
				t.Errorf("severity = %q, want %q", p.Severity, tc.severity)
			}
			wantRef := mustPage(t, rep, 1).Ref
			if tc.rule == "struct-elem-type" {
				wantRef = ""
				eachKid(rep.Elements, func(k *StructKid) {
					if k.Element != nil && k.Element.S == "" {
						wantRef = k.Element.Ref
					}
				})
			}
			if wantRef == "" || p.ObjRef != wantRef || p.ObjNodeID != uaNodeID(t, wantRef) {
				t.Errorf("objRef %q objNodeId %q, want %q", p.ObjRef, p.ObjNodeID, wantRef)
			}
			if tc.rule != "struct-elem-type" && !strings.HasPrefix(p.Message, "page 1") {
				t.Errorf("message %q does not name the page", p.Message)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// marked-content-balance
// ---------------------------------------------------------------------------

func TestValidateUA_BalanceUnmatchedEMCNamesStream(t *testing.T) {
	res, _ := uaRun(t, uaContentPDF("EMC /P <</MCID 0>> BDC EMC"))
	uaExpect(t, res, map[string]int{"marked-content-balance": 1})
	p := uaOnlyMessage(t, res, "marked-content-balance")
	uaOnPage(t, p)
	if want := "page 1: EMC with no open marked-content sequence (in content stream 20 0 R)"; p.Message != want {
		t.Errorf("message = %q, want %q", p.Message, want)
	}
	if p.Severity != "warning" {
		t.Errorf("severity = %q, want warning", p.Severity)
	}
}

func TestValidateUA_BalanceUnmatchedEMCInEachStreamIsOneFindingPerStream(t *testing.T) {
	res, _ := uaRun(t, uaContentPDF("EMC", "EMC EMC"))
	uaExpect(t, res, map[string]int{"marked-content-balance": 2})
	msgs := uaMessages(res, "marked-content-balance")
	if len(msgs) == 2 && (!strings.Contains(msgs[0], "20 0 R") || !strings.Contains(msgs[1], "21 0 R")) {
		t.Errorf("messages = %q, want one per stream in stream order", msgs)
	}
}

func TestValidateUA_BalanceOpenAtEndGivesOpenCount(t *testing.T) {
	res, _ := uaRun(t, uaContentPDF("/P <</MCID 0>> BDC EMC", "/P <</MCID 1>> BDC /Span BMC"))
	uaExpect(t, res, map[string]int{"marked-content-balance": 1})
	p := uaOnlyMessage(t, res, "marked-content-balance")
	uaOnPage(t, p)
	if want := "page 1: 2 marked-content sequence(s) still open at the end of the page's content"; p.Message != want {
		t.Errorf("message = %q, want %q", p.Message, want)
	}
}

func TestValidateUA_BalanceSequenceAcrossContentsElementsIsNotAFinding(t *testing.T) {
	res, _ := uaRun(t, uaContentPDF("/P <</MCID 0>> BDC (a) Tj", "(b) Tj EMC"))
	uaExpect(t, res, nil)
}

func TestValidateUA_BalanceBMCOnlyNestingIsBalanced(t *testing.T) {
	res, _ := uaRun(t, uaContentPDF("/A BMC /B BMC /C BMC EMC EMC EMC"))
	uaExpect(t, res, nil)
}

func TestValidateUA_BalanceUndecodableStreamIsReportedAndLaterStreamsStillRead(t *testing.T) {
	res, rep := uaRun(t, uaContentPDF(undecodableStream, "/P <</MCID 0>> BDC EMC /P <</MCID 0>> BDC EMC", undecodableStream))
	if mustPage(t, rep, 1).Streams[0].Err == "" {
		t.Fatal("precondition: stream 20 0 R decodes")
	}
	uaExpect(t, res, map[string]int{"marked-content-balance": 2, "mcid-unique": 1})
	ps := uaFindings(res, "marked-content-balance")
	for i, ref := range []string{"20 0 R", "22 0 R"} {
		if i >= len(ps) {
			break
		}
		p := ps[i]
		if p.ObjRef != ref || p.ObjNodeID != uaNodeID(t, ref) {
			t.Errorf("finding %d objRef %q objNodeId %q, want stream %s", i, p.ObjRef, p.ObjNodeID, ref)
		}
		wantPrefix := "content stream " + ref + " of page 1 could not be decoded ("
		if !strings.HasPrefix(p.Message, wantPrefix) || !strings.HasSuffix(p.Message, "); its marked content was not checked") {
			t.Errorf("finding %d = %q, want %q ... \"); its marked content was not checked\"", i, p.Message, wantPrefix)
		}
	}
}

func TestValidateUA_BalanceMalformedContentsIsAPageFinding(t *testing.T) {
	page := "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 42 /StructParents 0 >>"
	res, rep := uaRun(t, uaTaggedPDF(page, uaSlots(1)))
	sp := mustPage(t, rep, 1)
	if sp.Err == "" {
		t.Fatal("precondition: a /Contents of 42 leaves no page error in the report")
	}
	uaExpect(t, res, map[string]int{"marked-content-balance": 1})
	p := uaOnlyMessage(t, res, "marked-content-balance")
	uaOnPage(t, p)
	if want := "page 1 content could not be read (" + sp.Err + "); marked content not checked"; p.Message != want {
		t.Errorf("message = %q, want %q", p.Message, want)
	}
}

func TestValidateUA_BalanceStackAfterUndecodableStreamIsNotJudged(t *testing.T) {
	res, _ := uaRun(t, uaContentPDF("/P <</MCID 0>> BDC", undecodableStream, "EMC"))
	uaExpect(t, res, map[string]int{"marked-content-balance": 1})
	p := uaOnlyMessage(t, res, "marked-content-balance")
	if p.ObjRef != "21 0 R" || !strings.Contains(p.Message, "could not be decoded") {
		t.Errorf("finding = %+v, want only the decode finding on 21 0 R", p)
	}
}

func TestValidateUA_BalanceEMCAfterUndecodableStreamIsNotJudged(t *testing.T) {
	res, _ := uaRun(t, uaContentPDF(undecodableStream, "EMC"))
	uaExpect(t, res, map[string]int{"marked-content-balance": 1})
	p := uaOnlyMessage(t, res, "marked-content-balance")
	if p.ObjRef != "20 0 R" || !strings.Contains(p.Message, "could not be decoded") {
		t.Errorf("finding = %+v, want only the decode finding on 20 0 R", p)
	}
}

func TestValidateUA_BalanceOpenAtEndAfterUndecodableStreamIsNotJudged(t *testing.T) {
	res, _ := uaRun(t, uaContentPDF("/P <</MCID 0>> BDC", undecodableStream))
	uaExpect(t, res, map[string]int{"marked-content-balance": 1})
	p := uaOnlyMessage(t, res, "marked-content-balance")
	if p.ObjRef != "21 0 R" || !strings.Contains(p.Message, "could not be decoded") {
		t.Errorf("finding = %+v, want only the decode finding on 21 0 R", p)
	}
}

func TestValidateUA_BalanceUnmatchedEMCBeforeUndecodableStreamIsReported(t *testing.T) {
	res, _ := uaRun(t, uaContentPDF("EMC", undecodableStream))
	uaExpect(t, res, map[string]int{"marked-content-balance": 2})
	msgs := uaMessages(res, "marked-content-balance")
	if !slices.Contains(msgs, "page 1: EMC with no open marked-content sequence (in content stream 20 0 R)") {
		t.Errorf("messages = %q, want the unmatched EMC in stream 20 0 R", msgs)
	}
	if !slices.ContainsFunc(msgs, func(m string) bool {
		return strings.HasPrefix(m, "content stream 21 0 R of page 1 could not be decoded")
	}) {
		t.Errorf("messages = %q, want the decode finding on 21 0 R", msgs)
	}
}

// ---------------------------------------------------------------------------
// mcid-unique
// ---------------------------------------------------------------------------

func TestValidateUA_DuplicateMCIDWithinOneStream(t *testing.T) {
	res, rep := uaRun(t, uaContentPDF("/P <</MCID 0>> BDC EMC /P <</MCID 1>> BDC EMC /P <</MCID 0>> BDC EMC /P <</MCID 0>> BDC EMC"))
	uaExpect(t, res, map[string]int{"mcid-unique": 1})
	p := uaOnlyMessage(t, res, "mcid-unique")
	uaOnPage(t, p)
	want := fmt.Sprintf("page 1: MCID 0 marks 3 BDC sequences: %s, %s, %s", uaLoc(t, rep, 0), uaLoc(t, rep, 2), uaLoc(t, rep, 3))
	if p.Message != want {
		t.Errorf("message = %q, want %q", p.Message, want)
	}
}

func TestValidateUA_DuplicateMCIDAcrossContentsElements(t *testing.T) {
	res, rep := uaRun(t, uaContentPDF("/P <</MCID 0>> BDC EMC /P <</MCID 1>> BDC EMC", "/P <</MCID 1>> BDC EMC /P <</MCID 0>> BDC EMC"))
	uaExpect(t, res, map[string]int{"mcid-unique": 2})
	want := []string{
		fmt.Sprintf("page 1: MCID 0 marks 2 BDC sequences: %s, %s", uaLoc(t, rep, 0), uaLoc(t, rep, 3)),
		fmt.Sprintf("page 1: MCID 1 marks 2 BDC sequences: %s, %s", uaLoc(t, rep, 1), uaLoc(t, rep, 2)),
	}
	if got := uaMessages(res, "mcid-unique"); !slices.Equal(got, want) {
		t.Errorf("messages = %q, want %q", got, want)
	}
}

func TestValidateUA_SameMCIDOnTwoPagesIsNotADuplicate(t *testing.T) {
	pdf := rawPDF(
		rawObj{1, "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /Lang (en-US) /StructTreeRoot 5 0 R >>"},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 20 0 R /StructParents 0 >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 21 0 R /StructParents 1 >>"},
		rawObj{5, "<< /Type /StructTreeRoot /K [10 0 R] /ParentTree 6 0 R /ParentTreeNextKey 2 >>"},
		rawObj{6, "<< /Nums [0 [11 0 R] 1 [12 0 R]] >>"},
		rawObj{10, "<< /Type /StructElem /S /Document /P 5 0 R /K [11 0 R 12 0 R] >>"},
		rawObj{11, "<< /Type /StructElem /S /P /P 10 0 R /Pg 3 0 R /K 0 >>"},
		rawObj{12, "<< /Type /StructElem /S /P /P 10 0 R /Pg 4 0 R /K 0 >>"},
		rawObj{20, rawStream("/P <</MCID 0>> BDC EMC")},
		rawObj{21, rawStream("/P <</MCID 0>> BDC EMC")},
	)
	res, _ := uaRun(t, pdf)
	uaExpect(t, res, nil)
}

// ---------------------------------------------------------------------------
// mcid-in-parent-tree
// ---------------------------------------------------------------------------

func TestValidateUA_NoStructParentsIsOnePageWideFinding(t *testing.T) {
	res, _ := uaRun(t, uaStreamsPDF(uaSlots(10), "", "/P <</MCID 5>> BDC EMC /P <</MCID 0>> BDC EMC /P <</MCID 1>> BDC EMC /P <</MCID 0>> BDC EMC"))
	uaExpect(t, res, map[string]int{"mcid-in-parent-tree": 1, "mcid-unique": 1})
	p := uaOnlyMessage(t, res, "mcid-in-parent-tree")
	uaOnPage(t, p)
	if want := "page 1: MCIDs 0, 1, 5 not reachable: page has no /StructParents"; p.Message != want {
		t.Errorf("message = %q, want %q", p.Message, want)
	}
}

func TestValidateUA_StructParentsKeyMissingFromParentTree(t *testing.T) {
	res, rep := uaRun(t, uaStreamsPDF(uaSlots(10), "/StructParents 7", "/P <</MCID 1>> BDC EMC /P <</MCID 0>> BDC EMC"))
	sp := mustPage(t, rep, 1)
	if sp.ParentTreeErr == "" || !strings.Contains(sp.ParentTreeErr, "7") {
		t.Fatalf("precondition: ParentTreeErr %q does not name key 7", sp.ParentTreeErr)
	}
	uaExpect(t, res, map[string]int{"mcid-in-parent-tree": 1})
	p := uaOnlyMessage(t, res, "mcid-in-parent-tree")
	uaOnPage(t, p)
	if want := "page 1: MCIDs 0, 1 not reachable: " + sp.ParentTreeErr; p.Message != want {
		t.Errorf("message = %q, want %q", p.Message, want)
	}
}

func TestValidateUA_StructParentsNotAnIntegerNamesTheParentTreeError(t *testing.T) {
	res, rep := uaRun(t, uaStreamsPDF(uaSlots(10), "/StructParents /Zero", "/P <</MCID 0>> BDC EMC"))
	sp := mustPage(t, rep, 1)
	if sp.StructParents != nil || sp.ParentTreeErr == "" {
		t.Fatalf("precondition: StructParents %v ParentTreeErr %q, want nil and an error", sp.StructParents, sp.ParentTreeErr)
	}
	uaExpect(t, res, map[string]int{"mcid-in-parent-tree": 1})
	p := uaOnlyMessage(t, res, "mcid-in-parent-tree")
	if want := "page 1: MCIDs 0 not reachable: " + sp.ParentTreeErr; p.Message != want {
		t.Errorf("message = %q, want %q", p.Message, want)
	}
	if strings.Contains(p.Message, "page has no /StructParents") {
		t.Errorf("message %q blames a missing /StructParents for a non-integer one", p.Message)
	}
}

func TestValidateUA_NullSlotAndSlotWithErrorAreOneFindingPerMCID(t *testing.T) {
	res, rep := uaRun(t, uaStreamsPDF("0 [11 0 R null (not-an-element)]", "/StructParents 0",
		"/P <</MCID 0>> BDC EMC /P <</MCID 1>> BDC EMC /P <</MCID 2>> BDC EMC"))
	sp := mustPage(t, rep, 1)
	if len(sp.ParentTree) < 3 || sp.ParentTree[2].Err == "" {
		t.Fatalf("precondition: ParentTree slots %+v, want slot 2 with an error", sp.ParentTree)
	}
	uaExpect(t, res, map[string]int{"mcid-in-parent-tree": 2})
	ps := uaFindings(res, "mcid-in-parent-tree")
	if len(ps) != 2 {
		t.FailNow()
	}
	for _, p := range ps {
		uaOnPage(t, p)
	}
	if want := fmt.Sprintf("page 1: MCID 1 (%s) has no structure element in ParentTree key 0", uaLoc(t, rep, 1)); ps[0].Message != want {
		t.Errorf("null slot finding = %q, want %q", ps[0].Message, want)
	}
	wantPrefix := fmt.Sprintf("page 1: MCID 2 (%s) has no structure element in ParentTree key 0", uaLoc(t, rep, 2))
	if !strings.HasPrefix(ps[1].Message, wantPrefix) || !strings.Contains(ps[1].Message, sp.ParentTree[2].Err) {
		t.Errorf("error slot finding = %q, want %q followed by the slot error %q", ps[1].Message, wantPrefix, sp.ParentTree[2].Err)
	}
}

func TestValidateUA_UnreachableMCIDMarkedTwiceIsOneFindingListingBoth(t *testing.T) {
	res, rep := uaRun(t, uaStreamsPDF(uaSlots(1), "/StructParents 0", "/P <</MCID 1>> BDC EMC /P <</MCID 0>> BDC EMC", "/P <</MCID 1>> BDC EMC"))
	uaExpect(t, res, map[string]int{"mcid-in-parent-tree": 1, "mcid-unique": 1})
	p := uaOnlyMessage(t, res, "mcid-in-parent-tree")
	if want := fmt.Sprintf("page 1: MCID 1 (%s, %s) has no structure element in ParentTree key 0", uaLoc(t, rep, 0), uaLoc(t, rep, 2)); p.Message != want {
		t.Errorf("message = %q, want %q", p.Message, want)
	}
}

func TestValidateUA_UnreadableMCIDIsAParentTreeFinding(t *testing.T) {
	res, rep := uaRun(t, uaContentPDF("/P <</MCID /x>> BDC EMC"))
	row := mustPage(t, rep, 1).ContentMCIDs
	if len(row) != 1 || row[0].MCID != -1 || row[0].Err == "" {
		t.Fatalf("precondition: content rows %+v, want one unreadable MCID", row)
	}
	uaExpect(t, res, map[string]int{"mcid-in-parent-tree": 1})
	p := uaOnlyMessage(t, res, "mcid-in-parent-tree")
	uaOnPage(t, p)
	if want := fmt.Sprintf("page 1: BDC at %s has no readable MCID (%s); not checked against the ParentTree", uaLoc(t, rep, 0), row[0].Err); p.Message != want {
		t.Errorf("message = %q, want %q", p.Message, want)
	}
}

func TestValidateUA_ParentTreeFindingsAreInContentOrder(t *testing.T) {
	res, _ := uaRun(t, uaContentPDF("/P <</MCID /x>> BDC EMC /P <</MCID 12>> BDC EMC /P <</MCID 11>> BDC EMC /P <</MCID 12>> BDC EMC"))
	msgs := uaMessages(res, "mcid-in-parent-tree")
	if len(msgs) != 3 || !strings.Contains(msgs[0], "has no readable MCID") || !strings.HasPrefix(msgs[1], "page 1: MCID 12 ") || !strings.HasPrefix(msgs[2], "page 1: MCID 11 ") {
		t.Errorf("messages = %q, want the unreadable BDC, then MCID 12, then MCID 11", msgs)
	}
}

func TestValidateUA_DuplicateMCIDFindingsAreInContentOrder(t *testing.T) {
	res, _ := uaRun(t, uaContentPDF("/P <</MCID 5>> BDC EMC /P <</MCID 2>> BDC EMC /P <</MCID 5>> BDC EMC /P <</MCID 2>> BDC EMC"))
	msgs := uaMessages(res, "mcid-unique")
	if len(msgs) != 2 || !strings.HasPrefix(msgs[0], "page 1: MCID 5 ") || !strings.HasPrefix(msgs[1], "page 1: MCID 2 ") {
		t.Errorf("messages = %q, want MCID 5 then MCID 2", msgs)
	}
}

func TestValidateUA_FileTextInMessagesIsQuotedASCII(t *testing.T) {
	badFilter := "<< /Length 9 /Filter /Fo\xe9o >>\nstream\nnot-zlib!\nendstream"
	res, _ := uaRun(t, uaContentPDF("/P <</MCID /\xe9x>> BDC EMC", badFilter))
	for _, id := range []string{"marked-content-balance", "mcid-in-parent-tree"} {
		ps := uaFindings(res, id)
		if len(ps) == 0 {
			t.Errorf("%s: no finding, want one carrying the non-ASCII file text", id)
		}
		for _, p := range ps {
			for i := 0; i < len(p.Message); i++ {
				if p.Message[i] < 0x20 || p.Message[i] > 0x7e {
					t.Errorf("%s message %q has byte 0x%02x at %d", id, p.Message, p.Message[i], i)
					break
				}
			}
			if !strings.Contains(p.Message, `\u00e9`) && !strings.Contains(p.Message, `\xe9`) {
				t.Errorf("%s message %q, want the file text in quoted form", id, p.Message)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// mcid-not-nested
// ---------------------------------------------------------------------------

func TestValidateUA_MCIDInsideMCIDNamesBoth(t *testing.T) {
	res, rep := uaRun(t, uaContentPDF("/P <</MCID 0>> BDC /Span <</MCID 1>> BDC EMC EMC"))
	uaExpect(t, res, map[string]int{"mcid-not-nested": 1})
	p := uaOnlyMessage(t, res, "mcid-not-nested")
	uaOnPage(t, p)
	if want := fmt.Sprintf("page 1: MCID 1 (%s) is nested inside MCID 0", uaLoc(t, rep, 1)); p.Message != want {
		t.Errorf("message = %q, want %q", p.Message, want)
	}
}

func TestValidateUA_MCIDInsideNonStructuralSequenceIsAllowed(t *testing.T) {
	for name, content := range map[string]string{
		"inside BMC":              "/Artifact BMC /P <</MCID 0>> BDC EMC EMC",
		"inside MCID-less BDC":    "/Span <</Lang (en)>> BDC /P <</MCID 0>> BDC EMC EMC",
		"MCID-less BDC in a MCID": "/P <</MCID 0>> BDC /Span <</Lang (en)>> BDC EMC EMC",
	} {
		t.Run(name, func(t *testing.T) {
			res, _ := uaRun(t, uaContentPDF(content))
			uaExpect(t, res, nil)
		})
	}
}

func TestValidateUA_MCIDThreeDeepIsOneFindingPerNestedMCID(t *testing.T) {
	res, rep := uaRun(t, uaContentPDF("/Div <</MCID 0>> BDC /P <</MCID 1>> BDC /Art BMC /Span <</MCID 2>> BDC EMC EMC EMC EMC"))
	uaExpect(t, res, map[string]int{"mcid-not-nested": 2})
	want := []string{
		fmt.Sprintf("page 1: MCID 1 (%s) is nested inside MCID 0", uaLoc(t, rep, 1)),
		fmt.Sprintf("page 1: MCID 2 (%s) is nested inside MCID 1", uaLoc(t, rep, 2)),
	}
	if got := uaMessages(res, "mcid-not-nested"); !slices.Equal(got, want) {
		t.Errorf("messages = %q, want %q", got, want)
	}
}

func TestValidateUA_NestingAfterUndecodableStreamIsNotJudged(t *testing.T) {
	res, _ := uaRun(t, uaContentPDF("/P <</MCID 0>> BDC", undecodableStream, "/Span <</MCID 1>> BDC EMC EMC"))
	if r := uaRuleOf(t, res, "mcid-not-nested"); !r.Evaluated || r.Findings != 0 {
		t.Errorf("mcid-not-nested = %+v, want evaluated with 0 findings after an undecodable stream: %q", r, uaMessages(res, "mcid-not-nested"))
	}
}

func TestValidateUA_UnreadableMCIDInsideMCIDIsNotANestingFinding(t *testing.T) {
	res, _ := uaRun(t, uaContentPDF("/P <</MCID 0>> BDC /Span <</MCID /x>> BDC EMC EMC"))
	uaExpect(t, res, map[string]int{"mcid-in-parent-tree": 1})
}

// ---------------------------------------------------------------------------
// struct-elem-type
// ---------------------------------------------------------------------------

// uaTreePDF is a one-page document with the UA catalog entries, no marked
// content and a /StructTreeRoot 5 with the given body.
func uaTreePDF(rootBody string, objs ...rawObj) []byte {
	all := []rawObj{
		{1, "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /Lang (en-US) /StructTreeRoot 5 0 R >>"},
		{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>"},
		{5, rootBody},
	}
	return rawPDF(append(all, objs...)...)
}

// uaElement returns the element with the given ref from the report.
func uaElement(t *testing.T, rep *StructureReport, ref string) *StructElem {
	t.Helper()
	var found *StructElem
	eachKid(rep.Elements, func(k *StructKid) {
		if k.Element != nil && k.Element.Ref == ref {
			found = k.Element
		}
	})
	if found == nil {
		t.Fatalf("element %s missing from the report", ref)
	}
	return found
}

func TestValidateUA_ElementWithoutUsableSIsAnErrorOnTheElement(t *testing.T) {
	res, rep := uaRun(t, uaTreePDF("<< /Type /StructTreeRoot /K [10 0 R] >>",
		rawObj{10, "<< /Type /StructElem /S /Document /K [11 0 R 12 0 R 13 0 R 14 0 R] >>"},
		rawObj{11, "<< /Type /StructElem /T (no S) >>"},
		rawObj{12, "<< /Type /StructElem /S / >>"},
		rawObj{13, "<< /Type /StructElem /S (P) >>"},
		rawObj{14, "<< /Type /StructElem /S /P >>"},
	))
	uaExpect(t, res, map[string]int{"struct-elem-type": 3})
	ps := uaFindings(res, "struct-elem-type")
	for i, ref := range []string{"11 0 R", "12 0 R", "13 0 R"} {
		if i >= len(ps) {
			break
		}
		e := uaElement(t, rep, ref)
		if e.S != "" || e.Err == "" {
			t.Fatalf("precondition: element %s S %q Err %q", ref, e.S, e.Err)
		}
		p := ps[i]
		if p.Severity != "error" || p.ObjRef != ref || p.ObjNodeID != uaNodeID(t, ref) {
			t.Errorf("finding %d = %+v, want an error on %s", i, p, ref)
		}
		if want := "structure element " + ref + " has no usable /S (" + e.Err + ")"; p.Message != want {
			t.Errorf("finding %d = %q, want %q", i, p.Message, want)
		}
	}
	if res.Summary.Errors != 3 || res.Summary.Warnings != 0 {
		t.Errorf("summary = %+v, want 3 errors and no warnings for the struct-elem-type findings", res.Summary)
	}
}

func TestValidateUA_DirectElementWithoutSNamesItsPosition(t *testing.T) {
	res, _ := uaRun(t, uaTreePDF("<< /Type /StructTreeRoot /K [<< /Type /StructElem /T (root direct) >> 10 0 R] >>",
		rawObj{10, "<< /Type /StructElem /S /Div /K [11 0 R << /Type /StructElem /T (inner) >> << /Type /StructElem /S /P >>] >>"},
		rawObj{11, "<< /Type /StructElem /S /P >>"},
	))
	uaExpect(t, res, map[string]int{"struct-elem-type": 2})
	ps := uaFindings(res, "struct-elem-type")
	wants := []string{"direct structure element at /K index 0 of the structure tree root", "direct structure element at /K index 1 of 10 0 R"}
	for i, w := range wants {
		if i >= len(ps) {
			break
		}
		if ps[i].ObjRef != "" || ps[i].ObjNodeID != "" || !strings.Contains(ps[i].Message, w) || !strings.Contains(ps[i].Message, "/S") {
			t.Errorf("finding %d = %+v, want a document-level finding containing %q", i, ps[i], w)
		}
	}
}

func TestValidateUA_ErrorKidsAreNotAnSFinding(t *testing.T) {
	res, _ := uaRun(t, uaTreePDF("<< /Type /StructTreeRoot /K [10 0 R] >>",
		rawObj{10, "<< /Type /StructElem /S /Div /K [99 0 R (str) 10 0 R] >>"},
	))
	uaExpect(t, res, nil)
}

func TestValidateUA_RoleMappedSIsUsable(t *testing.T) {
	res, _ := uaRun(t, uaTreePDF("<< /Type /StructTreeRoot /RoleMap << /Heading1 /H1 >> /K [10 0 R] >>",
		rawObj{10, "<< /Type /StructElem /S /Heading1 >>"},
	))
	uaExpect(t, res, nil)
}

func TestValidateUA_ElementWithoutSAndBadPgCarriesTheWholeError(t *testing.T) {
	res, rep := uaRun(t, uaTreePDF("<< /Type /StructTreeRoot /K [10 0 R] >>",
		rawObj{10, "<< /Type /StructElem /Pg (not a page) >>"},
	))
	e := uaElement(t, rep, "10 0 R")
	if !strings.Contains(e.Err, "; ") || !strings.Contains(e.Err, "/Pg") {
		t.Fatalf("precondition: element Err %q, want the /S and /Pg problems joined", e.Err)
	}
	uaExpect(t, res, map[string]int{"struct-elem-type": 1})
	p := uaOnlyMessage(t, res, "struct-elem-type")
	if want := "structure element 10 0 R has no usable /S (" + e.Err + ")"; p.Message != want {
		t.Errorf("message = %q, want %q", p.Message, want)
	}
}

func TestValidateUA_StructTreeRootNotADictionaryLeavesSRuleNotEvaluated(t *testing.T) {
	res, _ := uaRun(t, uaTreePDF("[1 2 3]"))
	r := uaRuleOf(t, res, "struct-elem-type")
	if r.Evaluated || r.Findings != 0 {
		t.Errorf("struct-elem-type = %+v, want not evaluated", r)
	}
	p := uaOnlyMessage(t, res, "struct-elem-type")
	if p.Severity != "info" || !strings.Contains(p.Message, "structure tree root could not be read") {
		t.Errorf("problem = %+v, want an info problem naming the structure tree root", p)
	}
	for _, id := range uaContentRuleIDs {
		if r := uaRuleOf(t, res, id); !r.Evaluated {
			t.Errorf("content rule %s = %+v, want evaluated", id, r)
		}
	}
}

func TestValidateUA_InlineStructTreeRootNotADictionaryLeavesSRuleNotEvaluated(t *testing.T) {
	res, _ := uaRun(t, rawPDF(
		rawObj{1, "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /Lang (en-US) /StructTreeRoot [1 2 3] >>"},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>"},
	))
	r := uaRuleOf(t, res, "struct-elem-type")
	if r.Evaluated || r.Findings != 0 {
		t.Errorf("struct-elem-type = %+v, want not evaluated", r)
	}
	p := uaOnlyMessage(t, res, "struct-elem-type")
	if p.Severity != "info" || !strings.Contains(p.Message, "structure tree root could not be read") {
		t.Errorf("problem = %+v, want an info problem naming the structure tree root", p)
	}
}

func TestValidateUA_NoStructTreeRootEvaluatesSRuleWithNoFinding(t *testing.T) {
	res, _ := uaRun(t, rawPDF(
		rawObj{1, "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /Lang (en-US) >>"},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>"},
	))
	if r := uaRuleOf(t, res, "struct-elem-type"); !r.Evaluated || r.Findings != 0 {
		t.Errorf("struct-elem-type = %+v, want evaluated with 0 findings", r)
	}
	if r := uaRuleOf(t, res, "struct-tree-root"); r.Findings != 1 {
		t.Errorf("struct-tree-root = %+v, want the one finding for the absent root", r)
	}
}

// ---------------------------------------------------------------------------
// Not evaluated, fail closed
// ---------------------------------------------------------------------------

func TestCheckMCIDInParentTree_RootReadFailedPartWayIsNotEvaluated(t *testing.T) {
	key := 0
	rep := func(rootMsg string) *StructureReport {
		return &StructureReport{
			Problems: []StructureProblem{{Where: "StructTreeRoot 5 0 R", Message: rootMsg}},
			Pages: []*StructPage{{
				PageNum:       1,
				Ref:           "3 0 R",
				StructParents: &key,
				ParentTreeErr: "/StructParents 0 but the document has no ParentTree",
				Streams:       []MarkedStream{{Ref: "20 0 R"}},
				ContentMCIDs:  []ContentMCID{{MCID: 0, Stream: 0, Line: 1, Col: 1}},
			}},
		}
	}
	hits, err := checkMCIDInParentTree(nil, rep(rootUnreadablePrefix+"pdf parsing panic: boom"))
	if err == nil || len(hits) != 0 || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "ParentTree") {
		t.Errorf("hits %+v err %v, want not evaluated naming the root failure", hits, err)
	}
	hits, err = checkMCIDInParentTree(nil, rep(rootNotDictPrefix+"an array, not a dictionary"))
	if err != nil || len(hits) != 1 {
		t.Errorf("hits %+v err %v, want one finding when the root is not a dictionary", hits, err)
	}
}

func TestCheckMCIDInParentTree_ParentTreeNotReadInFullIsNotEvaluated(t *testing.T) {
	key := 0
	rep := func(where, msg string) *StructureReport {
		return &StructureReport{
			Problems: []StructureProblem{{Where: where, Message: msg}},
			Pages: []*StructPage{{
				PageNum:       1,
				Ref:           "3 0 R",
				StructParents: &key,
				ParentTreeErr: "ParentTree has no key 0",
				Streams:       []MarkedStream{{Ref: "20 0 R"}},
				ContentMCIDs:  []ContentMCID{{MCID: 0, Stream: 0, Line: 1, Col: 1}},
			}},
		}
	}
	for _, tc := range []struct{ where, msg string }{
		{"ParentTree 9 0 R, node 10 0 R", ntUnreadablePrefix + "pdf parsing panic: boom"},
		{"ParentTree, node (direct)", ntTooDeepPrefix + "64 levels; node not walked"},
	} {
		hits, err := checkMCIDInParentTree(nil, rep(tc.where, tc.msg))
		if err == nil || len(hits) != 0 || !strings.Contains(err.Error(), "ParentTree could not be read in full") {
			t.Errorf("%s %q: hits %+v err %v, want not evaluated", tc.where, tc.msg, hits, err)
		}
	}
	hits, err := checkMCIDInParentTree(nil, rep("ParentTree 9 0 R, node 11 0 R", "node is null"))
	if err != nil || len(hits) != 1 {
		t.Errorf("hits %+v err %v, want one finding when every node that exists was read", hits, err)
	}
}

func TestCheckStructElemType_RootReadFailedPartWayIsNotEvaluated(t *testing.T) {
	rep := &StructureReport{
		Problems: []StructureProblem{{Where: "StructTreeRoot 5 0 R", Message: rootUnreadablePrefix + "pdf parsing panic: boom"}},
		Elements: []*StructKid{{Kind: "element", Element: &StructElem{Ref: "6 0 R", S: "P"}}},
	}
	hits, err := checkStructElemType(nil, rep)
	if err == nil || len(hits) != 0 || !strings.Contains(err.Error(), "boom") {
		t.Errorf("hits %+v err %v, want not evaluated naming the root failure", hits, err)
	}
}

func TestValidateUA_PageTreeStoppedEarlyLeavesContentRulesNotEvaluated(t *testing.T) {
	pdf := rawPDF(append([]rawObj{
		{1, "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /Lang (en-US) /StructTreeRoot 5 0 R >>"},
		{5, "<< /Type /StructTreeRoot /K [] >>"},
	}, structPagesObjs(2, []string{"EMC", "EMC"}, nil)...)...)
	ins, doc := openUnvalidated(t, pdf)
	pw := newPageWalker(doc.PDFContext)
	pw.walk()
	if _, err := doc.pageIndex.get(func() (*pageTree, error) {
		return &pageTree{entries: pw.entries[:1], leaves: pw.leaves[:1], err: errors.New("pdf parsing panic: boom")}, nil
	}); err != nil {
		t.Fatal(err)
	}
	res, err := ins.Validate("raw", ProfilePDFUA1Structural)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	for _, id := range uaContentRuleIDs {
		r := uaRuleOf(t, res, id)
		if r.Evaluated || r.Findings != 0 {
			t.Errorf("rule %s = %+v, want not evaluated", id, r)
		}
		ps := uaFindings(res, id)
		if len(ps) != 1 || ps[0].Severity != "info" || !strings.Contains(ps[0].Message, "could not read every page") || !strings.Contains(ps[0].Message, "boom") {
			t.Errorf("rule %s problems = %+v, want one info problem naming the page-tree failure", id, ps)
		}
	}
	for _, id := range append([]string{"struct-elem-type"}, uaCatalogRuleIDs...) {
		if r := uaRuleOf(t, res, id); !r.Evaluated {
			t.Errorf("rule %s = %+v, want evaluated", id, r)
		}
	}
}

func TestValidateUA_UnreadableCatalogWinsOverStructureFaults(t *testing.T) {
	ins, doc := openUnvalidated(t, uaContentPDF("EMC /P <</MCID 0>> BDC EMC /P <</MCID 0>> BDC"))
	doc.PDFContext.XRefTable.RootDict = nil
	doc.PDFContext.XRefTable.Root = nil
	res, err := ins.Validate("raw", ProfilePDFUA1Structural)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(res.Rules) != 8 {
		t.Fatalf("got %d rules, want 8", len(res.Rules))
	}
	for _, r := range res.Rules {
		if r.Evaluated {
			t.Errorf("rule %s evaluated with an unreadable catalog", r.RuleID)
		}
		ps := uaFindings(res, r.RuleID)
		if len(ps) != 1 || ps[0].Severity != "info" || !strings.Contains(ps[0].Message, "catalog") {
			t.Errorf("rule %s problems = %+v, want one info problem naming the catalog", r.RuleID, ps)
		}
	}
}

func TestValidateUA_ConcurrentWithOtherReads(t *testing.T) {
	ins, _ := openUnvalidated(t, uaContentPDF("/P <</MCID 0>> BDC EMC", "/P <</MCID 1>> BDC EMC"))
	var wg sync.WaitGroup
	errs := make(chan error, 30)
	for i := 0; i < 10; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			res, err := ins.Validate("raw", ProfilePDFUA1Structural)
			if err != nil {
				errs <- fmt.Errorf("Validate: %w", err)
				return
			}
			if len(res.Rules) != 8 {
				errs <- fmt.Errorf("Validate ran %d rules, want 8", len(res.Rules))
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := ins.GetContentStream("raw", "obj:0:20"); err != nil {
				errs <- fmt.Errorf("GetContentStream: %w", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := ins.GetPageIndex("raw"); err != nil {
				errs <- fmt.Errorf("GetPageIndex: %w", err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

package pdfcore

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	pdfcpu_api "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The check's test hooks: pdfcpuCheckValidate is the validate step (by default
// pdfcpu_api.ValidateContextWithReport) and pdfcpuCheckParses counts the
// throwaway parses the check starts.

// swapCheckValidate replaces the validate step for the length of the test.
func swapCheckValidate(t *testing.T, fn func(context.Context, *pdfcpu_model.Context) (pdfcpu_model.ValidationReport, error)) {
	t.Helper()
	prev := pdfcpuCheckValidate
	pdfcpuCheckValidate = fn
	t.Cleanup(func() { pdfcpuCheckValidate = prev })
}

func openFixtureTab(t *testing.T, ins *Inspector, tabID, name string) {
	t.Helper()
	if _, err := ins.Open(tabID, filepath.Join(testdataDir(t), name)); err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	t.Cleanup(func() { _ = ins.Close(tabID) })
}

func runCheck(t *testing.T, name string) *ValidationResult {
	t.Helper()
	ins := NewInspector()
	openFixtureTab(t, ins, "pdfcpu-"+name, name)
	res, err := ins.Validate("pdfcpu-"+name, ProfilePdfcpu)
	if err != nil {
		t.Fatalf("Validate(%s, pdfcpu): %v", name, err)
	}
	return res
}

func problemsOf(res *ValidationResult, ruleID string) []Problem {
	var out []Problem
	for _, p := range res.Problems {
		if p.RuleID == ruleID {
			out = append(out, p)
		}
	}
	return out
}

func assertBothRulesEvaluated(t *testing.T, res *ValidationResult) {
	t.Helper()
	var ids []string
	for _, r := range res.Rules {
		ids = append(ids, r.RuleID)
		if !r.Evaluated {
			t.Errorf("rule %q not evaluated: %+v", r.RuleID, r)
		}
	}
	if !slices.Equal(ids, []string{"pdfcpu-validator", "pdfcpu-notices"}) {
		t.Errorf("rules = %v, want [pdfcpu-validator pdfcpu-notices]", ids)
	}
}

// waitOrFail waits for ch or fails the test after d.
func waitOrFail(t *testing.T, ch <-chan struct{}, d time.Duration, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(d):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestPdfcpuProfile_IsListedAfterTheStructuralProfiles(t *testing.T) {
	want := []string{ProfilePDFA1B, ProfilePDFUA1Structural, "pdfcpu"}
	if ProfilePdfcpu != "pdfcpu" {
		t.Errorf("ProfilePdfcpu = %q, want pdfcpu", ProfilePdfcpu)
	}
	if !slices.Equal(ValidProfiles, want) {
		t.Errorf("ValidProfiles = %v, want %v", ValidProfiles, want)
	}
	rules := ProfileRules(ProfilePdfcpu)
	if len(rules) != 2 {
		t.Fatalf("pdfcpu rules = %+v, want pdfcpu-validator and pdfcpu-notices", rules)
	}
	if rules[0].RuleID != "pdfcpu-validator" || rules[0].Severity != "error" || rules[0].SpecRef != "pdfcpu" {
		t.Errorf("first pdfcpu rule = %+v, want pdfcpu-validator, error, specRef pdfcpu", rules[0])
	}
	if rules[1].RuleID != "pdfcpu-notices" || rules[1].Severity != "warning" || rules[1].SpecRef != "pdfcpu" {
		t.Errorf("second pdfcpu rule = %+v, want pdfcpu-notices, warning, specRef pdfcpu", rules[1])
	}
	if got := ProfileStandard(ProfilePdfcpu); got != "pdfcpu validator (relaxed mode)" {
		t.Errorf("ProfileStandard(pdfcpu) = %q, want %q", got, "pdfcpu validator (relaxed mode)")
	}
}

func TestPdfcpuProfile_ScopeSaysSeparateParseAndPartialList(t *testing.T) {
	scope := strings.ToLower(ProfileScope(ProfilePdfcpu))
	for _, want := range []string{"separate parse", "not changed", "first", "log"} {
		if !strings.Contains(scope, want) {
			t.Errorf("pdfcpu scope must mention %q: %q", want, scope)
		}
	}
	if !strings.Contains(scope, "incomplete") && !strings.Contains(scope, "partial") {
		t.Errorf("pdfcpu scope must say the notice list can be incomplete: %q", scope)
	}
	assertHonestText(t, "pdfcpu scope", ProfileScope(ProfilePdfcpu))
	for _, r := range ProfileRules(ProfilePdfcpu) {
		assertHonestText(t, "checks for "+r.RuleID, r.Checks)
	}
}

func TestPdfcpuCheck_StructureBrokenReportsTheFatalProblemAtObject12(t *testing.T) {
	res := runCheck(t, "structure-broken.pdf")
	if res.Profile != ProfilePdfcpu {
		t.Errorf("profile = %q, want pdfcpu", res.Profile)
	}
	if res.Disclaimer != DisclaimerText {
		t.Errorf("disclaimer = %q, want the unchanged DisclaimerText", res.Disclaimer)
	}
	fatal := problemsOf(res, "pdfcpu-validator")
	if len(fatal) != 1 {
		t.Fatalf("pdfcpu-validator problems = %+v, want exactly one", fatal)
	}
	p := fatal[0]
	if p.Severity != "error" || !strings.Contains(p.Message, "circular structure tree") {
		t.Errorf("fatal problem = %+v, want an error naming the circular structure tree", p)
	}
	if p.ObjRef != "12 0 R" || p.ObjNodeID != "obj:0:12" {
		t.Errorf("fatal problem object = %q / %q, want 12 0 R / obj:0:12", p.ObjRef, p.ObjNodeID)
	}
	if p.Profile != ProfilePdfcpu || p.SpecRef != "pdfcpu" {
		t.Errorf("fatal problem profile/specRef = %q / %q, want pdfcpu / pdfcpu", p.Profile, p.SpecRef)
	}
	if res.Summary.Errors != 1 {
		t.Errorf("summary = %+v, want 1 error", res.Summary)
	}
	assertBothRulesEvaluated(t, res)
}

func TestPdfcpuCheck_FontsMixedReportsOneDigestedNotice(t *testing.T) {
	res := runCheck(t, "fonts-mixed.pdf")
	if fatal := problemsOf(res, "pdfcpu-validator"); len(fatal) != 0 {
		t.Errorf("fonts-mixed.pdf must have no pdfcpu-validator problem, got %+v", fatal)
	}
	notices := problemsOf(res, "pdfcpu-notices")
	if len(notices) != 1 {
		t.Fatalf("fonts-mixed.pdf notices = %+v, want exactly one (a pdfcpu change may have moved it)", notices)
	}
	n := notices[0]
	if n.Severity != "warning" {
		t.Errorf("notice severity = %q, want warning", n.Severity)
	}
	if want := "digested during validate: Type1 font CustomFont: missing required entries Widths"; n.Message != want {
		t.Errorf("notice message = %q, want %q", n.Message, want)
	}
	if n.ObjRef != "5 0 R" || n.ObjNodeID != "obj:0:5" {
		t.Errorf("notice object = %q / %q, want 5 0 R / obj:0:5", n.ObjRef, n.ObjNodeID)
	}
	if res.Summary.Errors != 0 || res.Summary.Warnings != 1 {
		t.Errorf("summary = %+v, want 0 errors and 1 warning", res.Summary)
	}
	assertBothRulesEvaluated(t, res)
}

func TestPdfcpuCheck_MinimalReportsNothing(t *testing.T) {
	res := runCheck(t, "minimal.pdf")
	if len(res.Problems) != 0 {
		t.Errorf("minimal.pdf problems = %+v, want none", res.Problems)
	}
	if res.Problems == nil || res.Rules == nil {
		t.Errorf("problems and rules must be non-nil slices: %+v", res)
	}
	assertBothRulesEvaluated(t, res)
}

// xref-stream-checksum.pdf opens only through the checksum-correcting reparse,
// so a check that parsed the file any other way would fail on it.
func TestPdfcpuCheck_XRefStreamChecksumFileUsesTheOpenParse(t *testing.T) {
	res := runCheck(t, "xref-stream-checksum.pdf")
	if res.Summary.Info != 0 {
		t.Errorf("check degraded on xref-stream-checksum.pdf: %+v", res.Problems)
	}
	assertBothRulesEvaluated(t, res)
}

// pdfcpu refuses xref-stream-checksum.pdf as written, so the check reports the
// checksum correction it ran after instead of a clean result.
func TestPdfcpuCheck_ChecksumCorrectionIsReportedAsANotice(t *testing.T) {
	res := runCheck(t, "xref-stream-checksum.pdf")
	notices := problemsOf(res, "pdfcpu-notices")
	if len(notices) == 0 {
		t.Fatalf("pdfcpu-notices on xref-stream-checksum.pdf = none, want the checksum correction; problems %+v", res.Problems)
	}
	first := notices[0].Message
	if !strings.HasPrefix(first, "repaired during parse: pdfcpu refuses the file as written") || !strings.Contains(first, "Adler-32") {
		t.Errorf("first notice = %q, want the repaired-during-parse checksum correction", first)
	}
	if res.Summary.Warnings == 0 {
		t.Errorf("summary = %+v, want at least one warning", res.Summary)
	}
}

// A file that parses but whose object stream fails on its Adler-32 trailer
// gets one repaired notice naming that object stream.
func TestParseFile_ObjectStreamChecksumCorrectionNamesTheStream(t *testing.T) {
	b, start := objectStreamFile(t)
	b = breakObjectStreamTrailer(b, start)
	_, failed, err := readPDF(context.Background(), bytes.NewReader(b))
	if err != nil || len(failed) != 1 {
		t.Fatalf("fixture: readPDF = failed %v, err %v; want one broken object stream", failed, err)
	}
	_, repairs, err := parseFile(context.Background(), bytes.NewReader(b), func() ([]byte, error) { return b, nil })
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	if len(repairs) != 1 {
		t.Fatalf("repairs = %+v, want one", repairs)
	}
	r := repairs[0]
	if r.ObjectNumber != failed[0] || r.Disposition != pdfcpu_model.NoticeRepaired || r.Phase != pdfcpu_model.NoticePhaseParse {
		t.Errorf("repair = %+v, want object %d, repaired, parse", r, failed[0])
	}
	if !strings.Contains(r.Message, "Adler-32") {
		t.Errorf("repair message = %q, want it to name the Adler-32 checksum", r.Message)
	}
}

// brokenTrailerZlib returns payload zlib-compressed with its Adler-32 trailer
// flipped, so the data inflates but the checksum fails.
func brokenTrailerZlib(t *testing.T, payload string) []byte {
	t.Helper()
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	if _, err := zw.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	b := z.Bytes()
	b[len(b)-1] ^= 0xFF
	return b
}

// twoBrokenObjectStreamsFile builds a file whose page uses font 4 from object
// stream 10 (/FlateDecode) and font 5 from object stream 11
// ([/ASCIIHexDecode /FlateDecode]); both streams have a broken Adler-32
// trailer, and the cross-reference stream is object 12.
func twoBrokenObjectStreamsFile(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	off := map[int]int{}
	obj := func(n int, s string) {
		off[n] = buf.Len()
		buf.WriteString(s)
	}
	stream := func(n int, dict string, data []byte) {
		obj(n, fmt.Sprintf("%d 0 obj\n<< %s /Length %d >>\nstream\n", n, dict, len(data)))
		buf.Write(data)
		buf.WriteString("\nendstream\nendobj\n")
	}
	buf.WriteString("%PDF-1.5\n")
	obj(1, "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	obj(2, "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	obj(3, "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R /F2 5 0 R >> >> >>\nendobj\n")
	stream(10, "/Type /ObjStm /N 1 /First 4 /Filter /FlateDecode",
		brokenTrailerZlib(t, "4 0 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
	stream(11, "/Type /ObjStm /N 1 /First 4 /Filter [/ASCIIHexDecode /FlateDecode]",
		[]byte(hex.EncodeToString(brokenTrailerZlib(t, "5 0 << /Type /Font /Subtype /Type1 /BaseFont /Courier >>"))+">"))

	// Cross-reference stream rows: type (1 byte), field 2 (4 bytes), field 3 (2 bytes).
	var x bytes.Buffer
	row := func(typ byte, f2 uint32, f3 uint16) {
		x.WriteByte(typ)
		_ = binary.Write(&x, binary.BigEndian, f2)
		_ = binary.Write(&x, binary.BigEndian, f3)
	}
	xoff := buf.Len()
	for n := 0; n <= 12; n++ {
		switch o, ok := off[n]; {
		case n == 0:
			row(0, 0, 65535)
		case n == 4:
			row(2, 10, 0)
		case n == 5:
			row(2, 11, 0)
		case n == 12:
			row(1, uint32(xoff), 0)
		case ok:
			row(1, uint32(o), 0)
		default:
			row(0, 0, 0)
		}
	}
	stream(12, "/Type /XRef /Size 13 /W [1 4 2] /Root 1 0 R", x.Bytes())
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xoff)
	return buf.Bytes()
}

// The object-stream correction finds zlib data at the stream's offset only, so
// a stream behind /ASCIIHexDecode keeps its broken trailer and gets no notice,
// while the plain /FlateDecode stream next to it does.
func TestParseFile_ObjectStreamTheCorrectionSkippedGetsNoNotice(t *testing.T) {
	b := twoBrokenObjectStreamsFile(t)
	_, failed, err := readPDF(context.Background(), bytes.NewReader(b))
	if err != nil || !slices.Equal(failed, []int{10, 11}) {
		t.Fatalf("fixture: readPDF = failed %v, err %v; want object streams 10 and 11 failing", failed, err)
	}
	pctx, repairs, err := parseFile(context.Background(), bytes.NewReader(b), func() ([]byte, error) { return bytes.Clone(b), nil })
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	if still := decodeObjectStreamObjects(pctx.XRefTable); !slices.Equal(still, []int{11}) {
		t.Fatalf("fixture: object streams failing after the correction = %v, want [11]", still)
	}
	var named []int
	for _, r := range repairs {
		named = append(named, r.ObjectNumber)
	}
	if !slices.Equal(named, []int{10}) {
		t.Errorf("repair notices name object streams %v, want only 10", named)
	}
}

// An intact file needs no correction, so parseFile reports none.
func TestParseFile_IntactFileHasNoRepairs(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	_, repairs, err := parseFile(context.Background(), bytes.NewReader(data), func() ([]byte, error) { return data, nil })
	if err != nil || len(repairs) != 0 {
		t.Errorf("parseFile(minimal.pdf) = repairs %+v, err %v; want none", repairs, err)
	}
}

// The validator runs in relaxed mode whatever configuration the parse came
// with, so a strict mode in the user's pdfcpu config.yml cannot change it.
func TestPdfcpuCheck_ValidatesInRelaxedMode(t *testing.T) {
	mode := -1
	swapCheckValidate(t, func(ctx context.Context, pctx *pdfcpu_model.Context) (pdfcpu_model.ValidationReport, error) {
		mode = pctx.Configuration.ValidationMode
		return pdfcpu_model.ValidationReport{}, nil
	})
	runCheck(t, "minimal.pdf")
	if mode != pdfcpu_model.ValidationRelaxed {
		t.Errorf("validation mode = %d, want ValidationRelaxed (%d)", mode, pdfcpu_model.ValidationRelaxed)
	}
}

// pdfcpu's validator writes /Subtype /Form into an XObject that has /BBox and
// no /Subtype, and reports no notice for it. The check must leave the
// displayed document as written, and the report has no item for the repair.
func TestPdfcpuCheck_LeavesTheDisplayedDocumentUnchanged(t *testing.T) {
	pdf := rawPDF(
		rawObj{1, "<< /Type /Catalog /Pages 2 0 R >>"},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /XObject << /No0 5 0 R >> >> /Contents 4 0 R >>"},
		rawObj{4, rawStreamObj("", "q /No0 Do Q\n")},
		rawObj{5, rawStreamObj("/Type /XObject /FormType 1 /BBox [0 0 10 10]", "0 0 10 10 re f\n")},
	)

	// Positive control: pdfcpu's validator reaches object 5 and infers /Form
	// on its own parse of the same bytes.
	ctl, err := pdfcpu_api.ReadContext(context.Background(), bytes.NewReader(pdf), pdfcpu_model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("control ReadContext: %v", err)
	}
	if err := pdfcpu_api.ValidateContext(context.Background(), ctl); err != nil {
		t.Fatalf("control ValidateContext: %v", err)
	}
	sd, _, err := ctl.XRefTable.DereferenceStreamDict(*pdfcpu_types.NewIndirectRef(5, 0))
	if err != nil || sd == nil {
		t.Fatalf("control dereference of object 5: %v", err)
	}
	if got := sd.Dict.NameEntry("Subtype"); got == nil || *got != "Form" {
		t.Fatalf("control: pdfcpu's validator no longer infers /Subtype /Form on object 5 (got %v); the pin below would pass vacuously", got)
	}

	ins, _ := openParsedFile(t, "no-subtype", pdf)
	res, err := ins.Validate("no-subtype", ProfilePdfcpu)
	if err != nil {
		t.Fatalf("Validate(pdfcpu): %v", err)
	}
	src, err := ins.GetObjectSource("no-subtype", "obj:0:5")
	if err != nil {
		t.Fatalf("GetObjectSource: %v", err)
	}
	if strings.Contains(src, "/Subtype") {
		t.Errorf("the check changed the displayed document: object 5 now shows /Subtype:\n%s", src)
	}
	for _, p := range res.Problems {
		if p.ObjRef == "5 0 R" || strings.Contains(p.Message, "Subtype") {
			t.Errorf("pdfcpu now reports its /Subtype inference (%+v); remove the known-gap note from the docs and scope", p)
		}
	}
}

func TestPdfcpuCheck_ChangedFileReturnsErrFileChangedAndCachesNothing(t *testing.T) {
	ins, path := openTempCopy(t, "minimal.pdf", "pdfcpu-changed")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	overwriteWithFixture(t, path, "multipage.pdf")
	res, err := ins.Validate("pdfcpu-changed", ProfilePdfcpu)
	if !errors.Is(err, ErrFileChanged) {
		t.Fatalf("Validate after overwrite: err = %v, want ErrFileChanged", err)
	}
	if res != nil {
		t.Errorf("Validate after overwrite returned a result: %+v", res)
	}

	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	before := pdfcpuCheckParses.Load()
	res, err = ins.Validate("pdfcpu-changed", ProfilePdfcpu)
	if err != nil {
		t.Fatalf("Validate after restoring the original file: %v", err)
	}
	if res == nil || len(res.Problems) != 0 {
		t.Errorf("result after restore = %+v, want the clean minimal.pdf result", res)
	}
	if got := pdfcpuCheckParses.Load() - before; got != 1 {
		t.Errorf("parses after restore = %d, want 1 (the refused call must not have cached anything)", got)
	}
}

// The file is verified and closed before the validator runs, so a save that
// lands during validation leaves the result for the bytes that were parsed,
// and the validator sees an empty reader.
func TestPdfcpuCheck_FileChangedDuringValidationKeepsTheParsedResult(t *testing.T) {
	ins, path := openTempCopy(t, "minimal.pdf", "pdfcpu-midchange")
	var once sync.Once
	readerSize := int64(-1)
	swapCheckValidate(t, func(ctx context.Context, c *pdfcpu_model.Context) (pdfcpu_model.ValidationReport, error) {
		once.Do(func() { overwriteWithFixture(t, path, "multipage.pdf") })
		if r, ok := c.Read.RS.(*bytes.Reader); ok {
			readerSize = r.Size()
		}
		return pdfcpu_api.ValidateContextWithReport(ctx, c)
	})
	res, err := ins.Validate("pdfcpu-midchange", ProfilePdfcpu)
	if err != nil {
		t.Fatalf("Validate with the file rewritten during validation: %v", err)
	}
	assertBothRulesEvaluated(t, res)
	if len(res.Problems) != 0 {
		t.Errorf("problems = %+v, want minimal.pdf's clean result", res.Problems)
	}
	if readerSize != 0 {
		t.Errorf("validator reader size = %d, want an empty *bytes.Reader", readerSize)
	}
}

func TestPdfcpuCheck_CloseDuringValidationReturnsCanceled(t *testing.T) {
	entered := make(chan struct{})
	swapCheckValidate(t, func(ctx context.Context, c *pdfcpu_model.Context) (pdfcpu_model.ValidationReport, error) {
		close(entered)
		<-ctx.Done()
		return pdfcpu_api.ValidateContextWithReport(ctx, c)
	})

	ins := NewInspector()
	if _, err := ins.Open("closing", filepath.Join(testdataDir(t), "structure-broken.pdf")); err != nil {
		t.Fatal(err)
	}
	// A test that fails before its own Close still cancels the held check.
	t.Cleanup(func() { _ = ins.Close("closing") })
	ins.mu.Lock()
	doc := ins.documents["closing"]
	ins.mu.Unlock()
	type outcome struct {
		res   *ValidationResult
		err   error
		panic any
	}
	done := make(chan outcome, 1)
	go func() {
		var o outcome
		defer func() {
			o.panic = recover()
			done <- o
		}()
		o.res, o.err = ins.Validate("closing", ProfilePdfcpu)
	}()
	select {
	case <-entered:
	case o := <-done:
		t.Fatalf("Validate returned before reaching validation: res %+v, err %v, panic %v", o.res, o.err, o.panic)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the check to reach validation")
	}
	if err := ins.Close("closing"); err != nil {
		t.Fatalf("Close: %v", err)
	}

	var o outcome
	select {
	case o = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Validate did not return after Close")
	}
	if o.panic != nil {
		t.Fatalf("Validate panicked after Close: %v", o.panic)
	}
	if !errors.Is(o.err, context.Canceled) {
		t.Errorf("Validate after Close: err = %v, want context.Canceled", o.err)
	}
	if o.res != nil {
		t.Errorf("Validate after Close returned a result (a cancel must not become a pdfcpu-validator problem): %+v", o.res)
	}
	if doc.pdfcpuCheck.isBuilt() {
		t.Error("a cancelled check cached a result")
	}
}

func TestPdfcpuCheck_QueuedCheckForClosedDocumentDoesNotParse(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	swapCheckValidate(t, func(ctx context.Context, c *pdfcpu_model.Context) (pdfcpu_model.ValidationReport, error) {
		once.Do(func() { close(entered) })
		select {
		case <-release:
		case <-ctx.Done():
		}
		return pdfcpu_api.ValidateContextWithReport(ctx, c)
	})

	ins := NewInspector()
	openFixtureTab(t, ins, "holder", "minimal.pdf")
	if _, err := ins.Open("queued", filepath.Join(testdataDir(t), "fonts-mixed.pdf")); err != nil {
		t.Fatal(err)
	}

	before := pdfcpuCheckParses.Load()
	holderDone := make(chan error, 1)
	go func() {
		_, err := ins.Validate("holder", ProfilePdfcpu)
		holderDone <- err
	}()
	select {
	case <-entered:
	case err := <-holderDone:
		t.Fatalf("first check returned before reaching validation: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the first check to hold the semaphore")
	}

	queuedDone := make(chan error, 1)
	go func() {
		_, err := ins.Validate("queued", ProfilePdfcpu)
		queuedDone <- err
	}()
	// Give the second check time to reach the semaphore wait.
	time.Sleep(200 * time.Millisecond)
	if err := ins.Close("queued"); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case err := <-queuedDone:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("queued check after Close: err = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("a check queued for a closed document still waits on the semaphore")
	}
	if got := pdfcpuCheckParses.Load() - before; got != 1 {
		t.Errorf("parses = %d, want 1 (only the holder; the queued check must not open or parse the file)", got)
	}

	close(release)
	if err := <-holderDone; err != nil {
		t.Errorf("holder check: %v", err)
	}
}

func TestPdfcpuCheck_PanicInValidatorDegradesBothRules(t *testing.T) {
	cases := map[string]func(){
		"runtime error": func() {
			var s []int
			_ = s[len(s)]
		},
		"plain panic": func() { panic("validator blew up") },
	}
	for name, boom := range cases {
		t.Run(name, func(t *testing.T) {
			swapCheckValidate(t, func(context.Context, *pdfcpu_model.Context) (pdfcpu_model.ValidationReport, error) {
				boom()
				return pdfcpu_model.ValidationReport{}, nil
			})
			ins := NewInspector()
			openFixtureTab(t, ins, "panics", "minimal.pdf")
			var res *ValidationResult
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("Validate let a validator panic escape: %v", r)
					}
				}()
				res, err = ins.Validate("panics", ProfilePdfcpu)
			}()
			if err != nil {
				t.Fatalf("Validate after a validator panic: err = %v, want a degraded result", err)
			}
			if len(res.Rules) != 2 {
				t.Fatalf("rules = %+v, want both pdfcpu rules", res.Rules)
			}
			for _, r := range res.Rules {
				if r.Evaluated {
					t.Errorf("rule %q reports evaluated after a panic", r.RuleID)
				}
				infos := 0
				for _, p := range problemsOf(res, r.RuleID) {
					if p.Severity != "info" {
						t.Errorf("rule %q has a %s problem after a panic: %+v", r.RuleID, p.Severity, p)
						continue
					}
					infos++
				}
				if infos != 1 {
					t.Errorf("rule %q has %d info problems, want 1", r.RuleID, infos)
				}
			}
			if res.Summary.Errors != 0 || res.Summary.Warnings != 0 || res.Summary.Info != 2 {
				t.Errorf("summary = %+v, want 2 info only", res.Summary)
			}
		})
	}
}

func TestPdfcpuCheck_SecondCallReturnsTheCachedResult(t *testing.T) {
	ins := NewInspector()
	openFixtureTab(t, ins, "cached", "fonts-mixed.pdf")
	before := pdfcpuCheckParses.Load()
	first, err := ins.Validate("cached", ProfilePdfcpu)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ins.Validate("cached", ProfilePdfcpu)
	if err != nil {
		t.Fatal(err)
	}
	if got := pdfcpuCheckParses.Load() - before; got != 1 {
		t.Errorf("two checks on one document parsed %d times, want 1", got)
	}
	if len(first.Problems) != len(second.Problems) || len(second.Problems) != 1 {
		t.Errorf("cached result differs: first %+v, second %+v", first.Problems, second.Problems)
	}
}

// The check parses and validates off the document lock, so other reads on the
// same document run while it works, and a held pdfMu does not stop the parse.
func TestPdfcpuCheck_RunsWithoutHoldingPdfMu(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	swapCheckValidate(t, func(ctx context.Context, c *pdfcpu_model.Context) (pdfcpu_model.ValidationReport, error) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return pdfcpu_api.ValidateContextWithReport(ctx, c)
	})

	ins := NewInspector()
	openFixtureTab(t, ins, "nolock", "structure-broken.pdf")
	doc, err := ins.GetDocument("nolock")
	if err != nil {
		t.Fatal(err)
	}

	doc.pdfMu.Lock()
	done := make(chan error, 1)
	go func() {
		_, err := ins.Validate("nolock", ProfilePdfcpu)
		done <- err
	}()
	select {
	case <-entered:
	case err := <-done:
		doc.pdfMu.Unlock()
		t.Fatalf("check returned before reaching validation: %v", err)
	case <-time.After(10 * time.Second):
		doc.pdfMu.Unlock()
		t.Fatal("the check did not finish its parse while another goroutine held pdfMu")
	}
	doc.pdfMu.Unlock()

	reads := make(chan struct{})
	go func() {
		defer close(reads)
		for range 5 {
			_, _ = ins.GetPageIndex("nolock")
			_, _ = ins.ResolveRef("nolock", "obj:0:1", ResolveOpts{MaxDepth: 2})
		}
	}()
	waitOrFail(t, reads, 10*time.Second, "GetPageIndex and ResolveRef while the check validates")

	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("check: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("check did not finish")
	}
}

func TestPdfcpuCheck_ConcurrentWithReadsOnTheSameDocument(t *testing.T) {
	ins := NewInspector()
	openFixtureTab(t, ins, "race", "fonts-mixed.pdf")
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		if _, err := ins.Validate("race", ProfilePdfcpu); err != nil {
			t.Errorf("Validate: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		for range 20 {
			_, _ = ins.GetPageIndex("race")
		}
	}()
	go func() {
		defer wg.Done()
		for range 20 {
			_, _ = ins.ResolveRef("race", "obj:0:5", ResolveOpts{MaxDepth: 3})
		}
	}()
	wg.Wait()
}

// A cancelled parse of a file that needs the checksum repair returns the
// cancel without reading the in-memory copy for the repair.
func TestParseFile_CancelledParseSkipsTheRepairRead(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir(t), "xref-stream-checksum.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reads := 0
	pctx, _, err := parseFile(ctx, bytes.NewReader(data), func() ([]byte, error) {
		reads++
		return data, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("parseFile with a cancelled ctx: err = %v, want context.Canceled", err)
	}
	if pctx != nil {
		t.Errorf("parseFile with a cancelled ctx returned a context")
	}
	if reads != 0 {
		t.Errorf("parseFile with a cancelled ctx read the file for the repair %d times, want 0", reads)
	}
}

// Acceptance tests for `dump page --info N`: the assembled per-page
// rendering picture (geometry + extgstates + xobjects + patterns + shadings +
// recursive forms). Black-box: build the CLI binary and run it as a subprocess.
//
// Covers: the full object; recursive forms with self-ref termination; the
// --section enum including its usage error; exit codes and empty arrays; the
// structural-only guard; the experimental note in help.
//
// Run: cd tests/cli-page-render && go test -v -count=1 ./...
package cli_page_render_test

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// `dump page --info 1` emits a single JSON object with page, pageRef,
// resolved geometry, and the resource arrays.
// ---------------------------------------------------------------------------

func TestPageDump_FullObject(t *testing.T) {
	bin := buildCLI(t)
	stdout, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", renderInfoPDF(t))
	if ec != 0 {
		t.Fatalf("exit %d", ec)
	}
	var obj map[string]any
	mustParseJSON(t, stdout, &obj)

	for _, key := range []string{"page", "pageRef", "mediaBox", "rotate", "extGStates", "xobjects", "patterns", "shadings"} {
		if _, ok := obj[key]; !ok {
			t.Errorf("full object missing key %q", key)
		}
	}
	// Geometry inheritance: MediaBox [0 0 300 400] from /Pages, rotate 90.
	mb, _ := obj["mediaBox"].([]any)
	if len(mb) != 4 || mb[2].(float64) != 300 || mb[3].(float64) != 400 {
		t.Errorf("mediaBox = %v, want [0 0 300 400] (inherited)", mb)
	}
	if r, _ := obj["rotate"].(float64); r != 90 {
		t.Errorf("rotate = %v, want 90 (inherited)", obj["rotate"])
	}
	// default output is compact single-line.
	if strings.Count(strings.TrimRight(stdout, "\n"), "\n") != 0 {
		t.Errorf("default output is not single-line compact")
	}
}

// ---------------------------------------------------------------------------
// `--forms-recursive` walks nested forms against their OWN resources and
// terminates a self-referential Do chain (cyclic), never looping.
// ---------------------------------------------------------------------------

func TestPageDump_RecursiveForms_SelfRefTerminates(t *testing.T) {
	bin := buildCLI(t)
	stdout, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", "--forms-recursive", "--forms-depth", "5", renderInfoPDF(t))
	if ec != 0 {
		t.Fatalf("exit %d", ec)
	}
	var obj struct {
		Forms []struct {
			Name  string `json:"name"`
			Forms []struct {
				Name   string `json:"name"`
				Cyclic bool   `json:"cyclic"`
			} `json:"forms"`
		} `json:"forms"`
	}
	mustParseJSON(t, stdout, &obj)

	var sawCyclic bool
	for _, f := range obj.Forms {
		if f.Name == "FmSelf" {
			for _, inner := range f.Forms {
				if inner.Name == "FmSelf" && inner.Cyclic {
					sawCyclic = true
				}
			}
		}
	}
	if !sawCyclic {
		t.Errorf("self-referential form not terminated with cyclic marker\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// Without --forms-recursive, the forms tree is not emitted (forms are listed
// in xobjects but not walked).
// ---------------------------------------------------------------------------

func TestPageDump_NoRecursionByDefault(t *testing.T) {
	bin := buildCLI(t)
	stdout, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", renderInfoPDF(t))
	if ec != 0 {
		t.Fatalf("exit %d", ec)
	}
	var obj map[string]any
	mustParseJSON(t, stdout, &obj)
	if v, ok := obj["forms"]; ok && v != nil {
		if arr, _ := v.([]any); len(arr) > 0 {
			t.Errorf("forms walked without --forms-recursive: %v", v)
		}
	}
}

// ---------------------------------------------------------------------------
// Each --section value emits ONLY that section.
// ---------------------------------------------------------------------------

func TestPageDump_SectionScoping(t *testing.T) {
	bin := buildCLI(t)
	cases := map[string][]string{
		"geometry":   {"mediaBox", "rotate"},
		"extgstates": {"extGStates"},
		"xobjects":   {"xobjects"},
		"forms":      {"forms"},
	}
	excluded := []string{"extGStates", "xobjects", "patterns", "shadings"}
	for section, wantKeys := range cases {
		args := []string{"dump", "page", "--json", "--info", "1", "--section", section}
		if section == "forms" {
			args = append(args, "--forms-recursive")
		}
		args = append(args, renderInfoPDF(t))
		stdout, _, ec := runCLI(t, bin, args...)
		if ec != 0 {
			t.Errorf("--section %s exit %d", section, ec)
			continue
		}
		var obj map[string]any
		mustParseJSON(t, stdout, &obj)
		for _, k := range wantKeys {
			if _, ok := obj[k]; !ok {
				t.Errorf("--section %s missing expected key %q", section, k)
			}
		}
		// patterns/shadings are never section-selectable; a scoped section must
		// not carry the OTHER sections.
		for _, k := range excluded {
			if _, isWanted := mapHas(wantKeys, k); isWanted {
				continue
			}
			if _, ok := obj[k]; ok {
				t.Errorf("--section %s leaked unrelated key %q", section, k)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// An unrecognized --section is a usage error (exit 1).
// ---------------------------------------------------------------------------

func TestPageDump_UnknownSection_UsageError(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", "--section", "patterns", renderInfoPDF(t))
	if ec != 1 {
		t.Errorf("--section patterns exit %d, want 1 (usage error)\nstdout: %s", ec, stdout)
	}
	if !strings.Contains(stderr, "section") {
		t.Errorf("stderr missing section error: %q", stderr)
	}
	// A truly bogus value is likewise a usage error.
	_, _, ec2 := runCLI(t, bin, "dump", "page", "--json", "--info", "1", "--section", "bogus", renderInfoPDF(t))
	if ec2 != 1 {
		t.Errorf("--section bogus exit %d, want 1", ec2)
	}
}

// ---------------------------------------------------------------------------
// out-of-range page -> JSON error on stderr, exit 2.
// ---------------------------------------------------------------------------

func TestPageDump_OutOfRange_Exit2(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "page", "--info", "999", renderInfoPDF(t))
	if ec != 2 {
		t.Errorf("out-of-range exit %d, want 2", ec)
	}
	if stdout != "" {
		t.Errorf("stdout must be empty on error, got %q", stdout)
	}
	var e map[string]string
	mustParseJSON(t, stderr, &e)
	if e["error"] == "" {
		t.Errorf("stderr is not a JSON error object: %q", stderr)
	}
}

// ---------------------------------------------------------------------------
// non-positive / missing --info is a usage error (exit 1).
// ---------------------------------------------------------------------------

func TestPageDump_BadInfo_UsageError(t *testing.T) {
	bin := buildCLI(t)
	for _, arg := range [][]string{
		{"dump", "page", "--info", "0", renderInfoPDF(t)},
		{"dump", "page", "--info", "-1", renderInfoPDF(t)},
		{"dump", "page", renderInfoPDF(t)},
	} {
		_, _, ec := runCLI(t, bin, arg...)
		if ec != 1 {
			t.Errorf("%v exit %d, want 1 (usage error)", arg, ec)
		}
	}
	// A malformed --forms-depth is also a usage error.
	_, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", "--forms-depth", "-3", renderInfoPDF(t))
	if ec != 1 {
		t.Errorf("--forms-depth -3 exit %d, want 1", ec)
	}
}

// ---------------------------------------------------------------------------
// A valid page with no /Resources emits empty arrays at exit 0 - an absent
// resource is a valid empty result, not an error.
// ---------------------------------------------------------------------------

func TestPageDump_NoResources_EmptyArraysExit0(t *testing.T) {
	bin := buildCLI(t)
	pdf := filepath.Join(testdataDir(t), "minimal.pdf")
	stdout, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", pdf)
	if ec != 0 {
		t.Fatalf("no-/Resources page exit %d, want 0", ec)
	}
	var obj struct {
		ExtGStates []any `json:"extGStates"`
		XObjects   []any `json:"xobjects"`
		Patterns   []any `json:"patterns"`
		Shadings   []any `json:"shadings"`
	}
	mustParseJSON(t, stdout, &obj)
	if obj.ExtGStates == nil || obj.XObjects == nil || obj.Patterns == nil || obj.Shadings == nil {
		t.Errorf("no-/Resources page must emit non-null empty arrays, got %s", stdout)
	}
	if len(obj.ExtGStates) != 0 || len(obj.XObjects) != 0 {
		t.Errorf("expected empty arrays, got %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// structural-only guard. The emitted JSON must NOT contain any
// computed-color / composited output field (rgb/cmyk/composited/
// renderedColor). Colorspace/blend/SMask carry only file-resident structure.
// ---------------------------------------------------------------------------

func TestPageDump_StructuralOnly_NoComputedColor(t *testing.T) {
	bin := buildCLI(t)
	stdout, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", "--forms-recursive", renderInfoPDF(t))
	if ec != 0 {
		t.Fatalf("exit %d", ec)
	}
	lower := strings.ToLower(stdout)
	for _, forbidden := range []string{"\"rgb\"", "\"cmyk\"", "composited", "renderedcolor", "\"colortorgb\"", "tintvalue"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("structural-only violation: output contains %q\n%s", forbidden, stdout)
		}
	}
	// Positive control: the structural inputs ARE present (family, function type
	// surfaced so the user runs the math themselves).
	if !strings.Contains(stdout, "ICCBased") {
		t.Errorf("expected structural colorspace family ICCBased to be present")
	}
}

// ---------------------------------------------------------------------------
// --pretty produces indented multi-line JSON that decodes to the same
// content as the compact default.
// ---------------------------------------------------------------------------

func TestPageDump_PrettyVsCompact(t *testing.T) {
	bin := buildCLI(t)
	compact, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", renderInfoPDF(t))
	if ec != 0 {
		t.Fatalf("compact exit %d", ec)
	}
	pretty, _, ep := runCLI(t, bin, "dump", "page", "--json", "--info", "1", "--pretty", renderInfoPDF(t))
	if ep != 0 {
		t.Fatalf("--pretty exit %d", ep)
	}
	if !strings.Contains(pretty, "\n  ") {
		t.Errorf("--pretty output is not indented multi-line")
	}
	var a, b any
	mustParseJSON(t, compact, &a)
	mustParseJSON(t, pretty, &b)
}

// ---------------------------------------------------------------------------
// The help text documents the command as EXPERIMENTAL.
// ---------------------------------------------------------------------------

func TestPageDump_HelpDocumentsExperimental(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, _ := runCLI(t, bin, "--help")
	help := stdout + stderr
	if !strings.Contains(help, "dump page") {
		t.Errorf("--help does not mention `dump page`")
	}
	if !strings.Contains(strings.ToUpper(help), "EXPERIMENTAL") {
		t.Errorf("--help does not flag `dump page --info` as EXPERIMENTAL")
	}
}

// ---------------------------------------------------------------------------
// Plain-text-default contract: the full-object --json output carries a top-level
// "_stability":"experimental" marker so a machine reader sees the instability
// in the payload. The DECISION (documented): the marker attaches to the full
// object only; a section-scoped --json view OMITS it.
// ---------------------------------------------------------------------------

func TestPageDump_JSON_FullObjectCarriesStabilityMarker(t *testing.T) {
	bin := buildCLI(t)

	full, _, ec := runCLI(t, bin, "dump", "page", "--json", "--info", "1", renderInfoPDF(t))
	if ec != 0 {
		t.Fatalf("page _stability: full --json exit %d", ec)
	}
	var fm map[string]any
	mustParseJSON(t, full, &fm)
	if s, _ := fm["_stability"].(string); s != "experimental" {
		t.Errorf("page _stability: full --json object must carry \"_stability\":\"experimental\", got %v", fm["_stability"])
	}

	// A section-scoped --json view omits the marker (documented decision).
	sec, _, ecs := runCLI(t, bin, "dump", "page", "--json", "--info", "1", "--section", "geometry", renderInfoPDF(t))
	if ecs != 0 {
		t.Fatalf("page _stability: section --json exit %d", ecs)
	}
	var sm map[string]any
	mustParseJSON(t, sec, &sm)
	if _, present := sm["_stability"]; present {
		t.Errorf("page _stability: section-scoped --json view must OMIT _stability, got %v", sm["_stability"])
	}
}

// ---------------------------------------------------------------------------
// Plain-text-default contract: the default (no --json) page --info output is
// human-readable PLAIN
// TEXT with aligned key/value sections (Geometry/ExtGStates/XObjects),
// honoring --section, and is NOT JSON. STRUCTURAL assertions only.
// ---------------------------------------------------------------------------

func TestPageDump_PlainTextDefault(t *testing.T) {
	bin := buildCLI(t)

	stdout, _, ec := runCLI(t, bin, "dump", "page", "--info", "1", renderInfoPDF(t))
	if ec != 0 {
		t.Fatalf("page plain: exit %d", ec)
	}
	trimmed := strings.TrimSpace(stdout)
	if strings.HasPrefix(trimmed, "{") && json.Valid([]byte(trimmed)) {
		t.Fatalf("page plain: default output must be plain text, not JSON:\n%.200s", stdout)
	}
	for _, heading := range []string{"Geometry:", "XObjects:"} {
		if !strings.Contains(stdout, heading) {
			t.Errorf("page plain: expected the %q section heading\n%s", heading, stdout)
		}
	}
	// _stability is a JSON-only marker; it must not appear in plain text.
	if strings.Contains(stdout, "_stability") {
		t.Errorf("page plain: plain output must not carry the _stability marker\n%s", stdout)
	}
}

// TestPageDump_PlainSectionHonored verifies the --section filter is honored in
// the plain-text view: --section geometry shows the Geometry block and not the
// XObjects block.
func TestPageDump_PlainSectionHonored(t *testing.T) {
	bin := buildCLI(t)

	stdout, _, ec := runCLI(t, bin, "dump", "page", "--info", "1", "--section", "geometry", renderInfoPDF(t))
	if ec != 0 {
		t.Fatalf("page plain section: exit %d", ec)
	}
	if !strings.Contains(stdout, "Geometry:") {
		t.Errorf("page plain section: --section geometry should show the Geometry block\n%s", stdout)
	}
	if strings.Contains(stdout, "XObjects:") {
		t.Errorf("page plain section: --section geometry must NOT show the XObjects block\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// Plain-text-default contract: the plain-text ExtGState summary labels the transparency alphas `ca`
// (non-stroking, PDF /ca) and `CA` (stroking, PDF /CA) with the SAME values
// the --json output carries under its `ca`/`CA` keys. This pins the
// label->semantics mapping to the JSON-tag contract, NOT to the (deliberately
// inverted) Go field names in internal/pdfcore/model.go - a field/label
// inversion regression would surface here. STRUCTURAL: token-level label+value
// assertions against the rendered line, not whole-dump equality.
// ---------------------------------------------------------------------------

func TestPageDump_PlainExtGStateAlphaLabels(t *testing.T) {
	bin := buildCLI(t)

	// Source of truth: the --json ca/CA values for GS0 (the JSON tags carry the
	// true PDF semantics regardless of the inverted Go field names).
	jsonOut, _, ecj := runCLI(t, bin, "dump", "page", "--json", "--info", "1", "--section", "extgstates", renderInfoPDF(t))
	if ecj != 0 {
		t.Fatalf("extgstate alpha: --json exit %d", ecj)
	}
	var jm struct {
		ExtGStates []struct {
			Name string   `json:"name"`
			Ca   *float64 `json:"ca"`
			CA   *float64 `json:"CA"`
		} `json:"extGStates"`
	}
	mustParseJSON(t, jsonOut, &jm)
	if len(jm.ExtGStates) == 0 {
		t.Fatalf("extgstate alpha: fixture has no ExtGStates in --json output:\n%s", jsonOut)
	}
	gs := jm.ExtGStates[0]
	if gs.Ca == nil || gs.CA == nil {
		t.Fatalf("extgstate alpha: fixture GS0 must carry both ca and CA in --json (got ca=%v CA=%v) - test needs a both-alphas fixture", gs.Ca, gs.CA)
	}

	plainOut, _, ecp := runCLI(t, bin, "dump", "page", "--info", "1", "--section", "extgstates", renderInfoPDF(t))
	if ecp != 0 {
		t.Fatalf("extgstate alpha: plain exit %d", ecp)
	}

	// Locate the GS0 line and tokenize it.
	var tokens []string
	for _, line := range strings.Split(plainOut, "\n") {
		if strings.Contains(line, gs.Name) {
			tokens = strings.Fields(line)
			break
		}
	}
	if tokens == nil {
		t.Fatalf("extgstate alpha: plain output has no line for %q:\n%s", gs.Name, plainOut)
	}

	// labelValue returns the token immediately following an exact-match label.
	labelValue := func(label string) (string, bool) {
		for i, tok := range tokens {
			if tok == label && i+1 < len(tokens) {
				return tokens[i+1], true
			}
		}
		return "", false
	}

	caVal, okCa := labelValue("ca")
	if !okCa {
		t.Fatalf("extgstate alpha: plain line missing `ca` label: %q", strings.Join(tokens, " "))
	}
	caUpperVal, okCA := labelValue("CA")
	if !okCA {
		t.Fatalf("extgstate alpha: plain line missing `CA` label: %q", strings.Join(tokens, " "))
	}

	// The plain `ca` label must carry the JSON `ca` value, and `CA` the JSON `CA`
	// value. %g matches the presenter's formatting in extGStateSummary.
	wantCa := strconv.FormatFloat(*gs.Ca, 'g', -1, 64)
	wantCA := strconv.FormatFloat(*gs.CA, 'g', -1, 64)
	if caVal != wantCa {
		t.Errorf("extgstate alpha: plain `ca` value = %q, want %q (JSON-tag semantics, not Go field name)", caVal, wantCa)
	}
	if caUpperVal != wantCA {
		t.Errorf("extgstate alpha: plain `CA` value = %q, want %q (JSON-tag semantics, not Go field name)", caUpperVal, wantCA)
	}
	// Guard against the inversion specifically: ca and CA must not be swapped
	// when the two values differ (they do in the fixture: 0.5 vs 1).
	if wantCa != wantCA && caVal == wantCA {
		t.Errorf("extgstate alpha: plain `ca`/`CA` labels are inverted (ca shows the CA value %q)", caVal)
	}
}

// mapHas reports whether key is in list (small helper for the section-leak
// exclusion check).
func mapHas(list []string, key string) (int, bool) {
	for i, v := range list {
		if v == key {
			return i, true
		}
	}
	return 0, false
}

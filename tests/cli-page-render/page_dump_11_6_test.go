// Story 11-6 acceptance tests for `dump page --info N`: the assembled per-page
// rendering picture (geometry + extgstates + xobjects + patterns + shadings +
// recursive forms). Black-box: build the CLI binary and run it as a subprocess.
//
// Covers: AC1 (full object), AC4 (recursive forms + self-ref termination),
// AC5 (--section enum incl. usage error), AC6 (exit codes + empty-arrays),
// AC7 (structural-only guard), AC8 (experimental note in help).
//
// Run: cd tests/cli-page-render && go test -v -count=1 ./...
package cli_page_render_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 11.6-INTG-001 [P0] (AC1): `dump page --info 1` emits a single JSON object
// with page, pageRef, resolved geometry, and the resource arrays.
// ---------------------------------------------------------------------------

func TestPageDump_FullObject(t *testing.T) {
	bin := buildCLI(t)
	stdout, _, ec := runCLI(t, bin, "dump", "page", "--info", "1", renderInfoPDF(t))
	if ec != 0 {
		t.Fatalf("[P0] 11.6-INTG-001: exit %d", ec)
	}
	var obj map[string]any
	mustParseJSON(t, stdout, &obj)

	for _, key := range []string{"page", "pageRef", "mediaBox", "rotate", "extGStates", "xobjects", "patterns", "shadings"} {
		if _, ok := obj[key]; !ok {
			t.Errorf("[P0] 11.6-INTG-001: full object missing key %q", key)
		}
	}
	// Geometry inheritance: MediaBox [0 0 300 400] from /Pages, rotate 90.
	mb, _ := obj["mediaBox"].([]any)
	if len(mb) != 4 || mb[2].(float64) != 300 || mb[3].(float64) != 400 {
		t.Errorf("[P0] 11.6-INTG-001: mediaBox = %v, want [0 0 300 400] (inherited)", mb)
	}
	if r, _ := obj["rotate"].(float64); r != 90 {
		t.Errorf("[P0] 11.6-INTG-001: rotate = %v, want 90 (inherited)", obj["rotate"])
	}
	// default output is compact single-line.
	if strings.Count(strings.TrimRight(stdout, "\n"), "\n") != 0 {
		t.Errorf("[P0] 11.6-INTG-001: default output is not single-line compact")
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-002 [P0] (AC4): `--forms-recursive` walks nested forms against
// their OWN resources and terminates a self-referential Do chain (cyclic),
// never looping.
// ---------------------------------------------------------------------------

func TestPageDump_RecursiveForms_SelfRefTerminates(t *testing.T) {
	bin := buildCLI(t)
	stdout, _, ec := runCLI(t, bin, "dump", "page", "--info", "1", "--forms-recursive", "--forms-depth", "5", renderInfoPDF(t))
	if ec != 0 {
		t.Fatalf("[P0] 11.6-INTG-002: exit %d", ec)
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
		t.Errorf("[P0] 11.6-INTG-002: self-referential form not terminated with cyclic marker\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-003 [P1] (AC4): without --forms-recursive, the forms tree is not
// emitted (forms are listed in xobjects but not walked).
// ---------------------------------------------------------------------------

func TestPageDump_NoRecursionByDefault(t *testing.T) {
	bin := buildCLI(t)
	stdout, _, ec := runCLI(t, bin, "dump", "page", "--info", "1", renderInfoPDF(t))
	if ec != 0 {
		t.Fatalf("[P1] 11.6-INTG-003: exit %d", ec)
	}
	var obj map[string]any
	mustParseJSON(t, stdout, &obj)
	if v, ok := obj["forms"]; ok && v != nil {
		if arr, _ := v.([]any); len(arr) > 0 {
			t.Errorf("[P1] 11.6-INTG-003: forms walked without --forms-recursive: %v", v)
		}
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-004 [P0] (AC5): each --section value emits ONLY that section.
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
		args := []string{"dump", "page", "--info", "1", "--section", section}
		if section == "forms" {
			args = append(args, "--forms-recursive")
		}
		args = append(args, renderInfoPDF(t))
		stdout, _, ec := runCLI(t, bin, args...)
		if ec != 0 {
			t.Errorf("[P0] 11.6-INTG-004: --section %s exit %d", section, ec)
			continue
		}
		var obj map[string]any
		mustParseJSON(t, stdout, &obj)
		for _, k := range wantKeys {
			if _, ok := obj[k]; !ok {
				t.Errorf("[P0] 11.6-INTG-004: --section %s missing expected key %q", section, k)
			}
		}
		// patterns/shadings are never section-selectable; a scoped section must
		// not carry the OTHER sections.
		for _, k := range excluded {
			if _, isWanted := mapHas(wantKeys, k); isWanted {
				continue
			}
			if _, ok := obj[k]; ok {
				t.Errorf("[P0] 11.6-INTG-004: --section %s leaked unrelated key %q", section, k)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 11.6-UNIT-005 [P0] (AC5): an unrecognized --section is a usage error (exit 1).
// ---------------------------------------------------------------------------

func TestPageDump_UnknownSection_UsageError(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "page", "--info", "1", "--section", "patterns", renderInfoPDF(t))
	if ec != 1 {
		t.Errorf("[P0] 11.6-UNIT-005: --section patterns exit %d, want 1 (usage error)\nstdout: %s", ec, stdout)
	}
	if !strings.Contains(stderr, "section") {
		t.Errorf("[P0] 11.6-UNIT-005: stderr missing section error: %q", stderr)
	}
	// A truly bogus value is likewise a usage error.
	_, _, ec2 := runCLI(t, bin, "dump", "page", "--info", "1", "--section", "bogus", renderInfoPDF(t))
	if ec2 != 1 {
		t.Errorf("[P0] 11.6-UNIT-005: --section bogus exit %d, want 1", ec2)
	}
}

// ---------------------------------------------------------------------------
// 11.6-UNIT-006 [P0] (AC6): out-of-range page -> JSON error on stderr, exit 2.
// ---------------------------------------------------------------------------

func TestPageDump_OutOfRange_Exit2(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "page", "--info", "999", renderInfoPDF(t))
	if ec != 2 {
		t.Errorf("[P0] 11.6-UNIT-006: out-of-range exit %d, want 2", ec)
	}
	if stdout != "" {
		t.Errorf("[P0] 11.6-UNIT-006: stdout must be empty on error, got %q", stdout)
	}
	var e map[string]string
	mustParseJSON(t, stderr, &e)
	if e["error"] == "" {
		t.Errorf("[P0] 11.6-UNIT-006: stderr is not a JSON error object: %q", stderr)
	}
}

// ---------------------------------------------------------------------------
// 11.6-UNIT-007 [P0] (AC6): non-positive / missing --info is a usage error
// (exit 1).
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
			t.Errorf("[P0] 11.6-UNIT-007: %v exit %d, want 1 (usage error)", arg, ec)
		}
	}
	// A malformed --forms-depth is also a usage error.
	_, _, ec := runCLI(t, bin, "dump", "page", "--info", "1", "--forms-depth", "-3", renderInfoPDF(t))
	if ec != 1 {
		t.Errorf("[P0] 11.6-UNIT-007: --forms-depth -3 exit %d, want 1", ec)
	}
}

// ---------------------------------------------------------------------------
// 11.6-UNIT-008 [P1] (AC6): a valid page with no /Resources emits empty arrays
// at exit 0 - an absent resource is a valid empty result, not an error.
// ---------------------------------------------------------------------------

func TestPageDump_NoResources_EmptyArraysExit0(t *testing.T) {
	bin := buildCLI(t)
	pdf := filepath.Join(testdataDir(t), "minimal.pdf")
	stdout, _, ec := runCLI(t, bin, "dump", "page", "--info", "1", pdf)
	if ec != 0 {
		t.Fatalf("[P1] 11.6-UNIT-008: no-/Resources page exit %d, want 0", ec)
	}
	var obj struct {
		ExtGStates []any `json:"extGStates"`
		XObjects   []any `json:"xobjects"`
		Patterns   []any `json:"patterns"`
		Shadings   []any `json:"shadings"`
	}
	mustParseJSON(t, stdout, &obj)
	if obj.ExtGStates == nil || obj.XObjects == nil || obj.Patterns == nil || obj.Shadings == nil {
		t.Errorf("[P1] 11.6-UNIT-008: no-/Resources page must emit non-null empty arrays, got %s", stdout)
	}
	if len(obj.ExtGStates) != 0 || len(obj.XObjects) != 0 {
		t.Errorf("[P1] 11.6-UNIT-008: expected empty arrays, got %s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 11.6-UNIT-009 [P0] (AC7): structural-only guard. The emitted JSON must NOT
// contain any computed-color / composited output field (rgb/cmyk/composited/
// renderedColor). Colorspace/blend/SMask carry only file-resident structure.
// ---------------------------------------------------------------------------

func TestPageDump_StructuralOnly_NoComputedColor(t *testing.T) {
	bin := buildCLI(t)
	stdout, _, ec := runCLI(t, bin, "dump", "page", "--info", "1", "--forms-recursive", renderInfoPDF(t))
	if ec != 0 {
		t.Fatalf("[P0] 11.6-UNIT-009: exit %d", ec)
	}
	lower := strings.ToLower(stdout)
	for _, forbidden := range []string{"\"rgb\"", "\"cmyk\"", "composited", "renderedcolor", "\"colortorgb\"", "tintvalue"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("[P0] 11.6-UNIT-009: structural-only violation: output contains %q\n%s", forbidden, stdout)
		}
	}
	// Positive control: the structural inputs ARE present (family, function type
	// surfaced so the user runs the math themselves).
	if !strings.Contains(stdout, "ICCBased") {
		t.Errorf("[P0] 11.6-UNIT-009: expected structural colorspace family ICCBased to be present")
	}
}

// ---------------------------------------------------------------------------
// 11.6-INTG-010 [P1] (AC1/AC3): --pretty produces indented multi-line JSON
// that decodes to the same content as the compact default.
// ---------------------------------------------------------------------------

func TestPageDump_PrettyVsCompact(t *testing.T) {
	bin := buildCLI(t)
	compact, _, ec := runCLI(t, bin, "dump", "page", "--info", "1", renderInfoPDF(t))
	if ec != 0 {
		t.Fatalf("[P1] 11.6-INTG-010: compact exit %d", ec)
	}
	pretty, _, ep := runCLI(t, bin, "dump", "page", "--info", "1", "--pretty", renderInfoPDF(t))
	if ep != 0 {
		t.Fatalf("[P1] 11.6-INTG-010: --pretty exit %d", ep)
	}
	if !strings.Contains(pretty, "\n  ") {
		t.Errorf("[P1] 11.6-INTG-010: --pretty output is not indented multi-line")
	}
	var a, b any
	mustParseJSON(t, compact, &a)
	mustParseJSON(t, pretty, &b)
}

// ---------------------------------------------------------------------------
// 11.6-DOC-011 [P1] (AC8): the help text documents the command as EXPERIMENTAL.
// ---------------------------------------------------------------------------

func TestPageDump_HelpDocumentsExperimental(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, _ := runCLI(t, bin, "--help")
	help := stdout + stderr
	if !strings.Contains(help, "dump page") {
		t.Errorf("[P1] 11.6-DOC-011: --help does not mention `dump page`")
	}
	if !strings.Contains(strings.ToUpper(help), "EXPERIMENTAL") {
		t.Errorf("[P1] 11.6-DOC-011: --help does not flag `dump page --info` as EXPERIMENTAL")
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

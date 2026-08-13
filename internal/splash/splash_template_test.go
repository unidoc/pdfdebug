package splash

import (
	"bytes"
	"html"
	"strings"
	"testing"
	"text/template"
)

// TestRenderUsesTextTemplate is the Story 10.9 red-phase contract for the
// splash.Render text/template migration (finding #27).
//
// RED PHASE: this test references the package-scope splashTmpl template
// variable, so Render must be backed by:
//
//	var splashTmpl = template.Must(template.New("splash").Parse(splashHTMLTemplate))
//
// That variable does not exist yet, so the splash package test binary FAILS TO
// COMPILE. This is the expected red state. The Dev step turns it green by:
//   1. adding the text/template import,
//   2. declaring splashTmpl at package scope,
//   3. replacing the strings.ReplaceAll call in Render with
//      splashTmpl.Execute(&buf, struct{ Version string }{ ... }).
//
// Beyond the compile-time symbol assertion, the test independently re-derives
// the expected text/template output and asserts Render's bytes are identical
// for the three Version inputs named in ("dev", "v0.2.0", "v0.2.0-rc1").
// The html.EscapeString(RenderVersion(version)) pre-escaping (Dev Notes for
// #27: text/template does NOT HTML-escape, so the explicit escape must remain)
// is reproduced here so the parity check is exact.
func TestRenderUsesTextTemplate(t *testing.T) {
	// Reference the package-scope splashTmpl the migration must introduce.
	// Undefined until the Dev step lands -> compile error -> red.
	if splashTmpl == nil {
		t.Fatal("splashTmpl is nil: Render must be backed by a package-scope text/template")
	}

	// Independent oracle: a freshly parsed text/template over the UNCHANGED
	// splashHTMLTemplate constant, executed against the same single-field
	// struct specifies. This is what Render must produce byte-for-byte.
	oracle := template.Must(template.New("splash-oracle").Parse(splashHTMLTemplate))

	for _, version := range []string{"dev", "v0.2.0", "v0.2.0-rc1"} {
		var buf bytes.Buffer
		if err := oracle.Execute(&buf, struct{ Version string }{Version: html.EscapeString(RenderVersion(version))}); err != nil {
			t.Fatalf("oracle template execute failed for %q: %v", version, err)
		}
		want := buf.String()

		got := Render(version)
		if got != want {
			t.Errorf("Render(%q) output is not byte-identical to text/template execution\n got len=%d\nwant len=%d", version, len(got), len(want))
		}

		// The placeholder must be fully substituted (no literal {{.Version}}
		// left behind by either path).
		if strings.Contains(got, "{{.Version}}") {
			t.Errorf("Render(%q) left the {{.Version}} placeholder unsubstituted", version)
		}

		// The rendered, HTML-escaped version must appear in the output.
		if escaped := html.EscapeString(RenderVersion(version)); !strings.Contains(got, escaped) {
			t.Errorf("Render(%q) output missing the rendered version %q", version, escaped)
		}
	}
}

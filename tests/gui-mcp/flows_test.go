//go:build guimcp && !windows

package guimcp_test

import (
	"runtime"
	"strings"
	"testing"
)

func TestOpenFixtureFromTheCommandLine(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("only Linux delivers cold-start command-line files to the app; macOS opens nothing from argv")
	}
	a := launch(t, fixture(t, "minimal.pdf"))
	a.waitFor("a document tab for minimal.pdf", func() bool { return containsName(a.tabNames(), "minimal.pdf") })
}

func TestOpenFixturesThroughASecondLaunch(t *testing.T) {
	a := launch(t)
	if a.count(`[data-testid="empty-state"]`) == 0 {
		t.Fatal("a fresh instance does not show the empty state")
	}
	a.openFixtures("minimal.pdf", "multipage.pdf")
	names := a.tabNames()
	if len(names) != 2 {
		t.Errorf("tab bar shows %v, want two tabs", names)
	}
	if a.count(`[data-testid="empty-state"]`) != 0 {
		t.Error("empty state still shown with two documents open")
	}
}

func TestExpandTreeNodeShowsItsChildren(t *testing.T) {
	a := launch(t)
	a.openFixtures("multipage.pdf")

	var target struct {
		Found bool    `json:"found"`
		X     float64 `json:"x"`
		Y     float64 `json:"y"`
		ID    string  `json:"id"`
	}
	a.mustJS(`
		for (const row of document.querySelectorAll('[data-testid="tree-node"]')) {
			const arrow = row.querySelector('span');
			if (arrow && arrow.textContent.trim() === '>') {
				const r = arrow.getBoundingClientRect();
				return {found: true, x: r.x + r.width / 2, y: r.y + r.height / 2, id: row.getAttribute('data-node-id') || ''};
			}
		}
		return {found: false};`, &target)
	if !target.Found {
		t.Fatal("no collapsed tree row with children is visible")
	}
	before := a.count(`[data-testid="tree-node"]`)
	a.clickAt(target.X, target.Y)
	a.waitFor("the expanded row's children to render", func() bool {
		n, err := a.tryCount(`[data-testid="tree-node"]`)
		return err == nil && n > before
	})
	var arrow string
	a.mustJS(`const row = document.querySelector('[data-testid="tree-node"][data-node-id="`+target.ID+`"]'); return row ? row.querySelector('span').textContent.trim() : '';`, &arrow)
	if arrow != "v" {
		t.Errorf("expanded row arrow = %q, want the open marker", arrow)
	}
}

func TestSignaturesTabShowsTheCertificateChain(t *testing.T) {
	a := launch(t)
	a.openFixtures("signed.pdf")

	a.waitFor("the Signatures tab for a signed document", func() bool { return a.visible(`[data-testid="detail-tab-signatures"]`) })
	a.click(`[data-testid="detail-tab-signatures"]`)
	a.waitFor("the Signatures pane", func() bool { return a.visible(`[data-testid="detail-pane-signatures"]`) })
	a.waitFor("the certificate chain control", func() bool { return a.visible(`[data-testid="signature-cert-chain"]`) })

	a.click(`[data-testid="signature-cert-chain"]`)
	a.waitFor("the certificate chain to expand", func() bool {
		var expanded string
		err := a.js(`const el = document.querySelector('[data-testid="signature-cert-chain"]'); return el ? el.getAttribute('aria-expanded') : '';`, &expanded)
		return err == nil && expanded == "true"
	})
}

func TestSwitchDetailTabs(t *testing.T) {
	a := launch(t)
	a.openFixtures("minimal.pdf")

	for _, view := range []string{"xref", "plaintext"} {
		a.click(`[data-testid="detail-tab-` + view + `"]`)
		a.waitFor("the "+view+" pane", func() bool { return a.visible(`[data-testid="detail-pane-` + view + `"]`) })
	}
	if a.visible(`[data-testid="detail-pane-xref"]`) {
		t.Error("xref pane still visible after switching to another detail tab")
	}
}

func TestSwitchDocuments(t *testing.T) {
	a := launch(t)
	a.openFixtures("minimal.pdf", "multipage.pdf")

	first := a.activeTabName()
	if first == "" {
		t.Fatal("no document tab is active")
	}
	a.click(`[data-testid="tab-list"] [role="tab"][data-state="inactive"]`)
	a.waitFor("the other document to become active", func() bool {
		now := a.activeTabName()
		return now != "" && now != first
	})
	now := a.activeTabName()
	if !strings.Contains(now, "minimal.pdf") && !strings.Contains(now, "multipage.pdf") {
		t.Errorf("active tab after switching = %q, want one of the opened fixtures", now)
	}
}

func TestCompareThroughTheDialogStub(t *testing.T) {
	a := launch(t)
	a.openFixtures("minimal.pdf")

	a.mustCall("call_bound_method", map[string]any{
		"name": "unidoc-pdf-debugger/internal/pdfservice.PDFService.QueueDialogPaths",
		"args": []any{[]string{fixture(t, "multipage.pdf")}},
	})
	a.click(`[data-testid="detail-tab-diff"]`)
	a.waitFor("Compare's Pick file control", func() bool { return a.visible(`[data-testid="diff-pick-file"]`) })
	a.click(`[data-testid="diff-pick-file"]`)
	// The diff view names only an on-disk right side, so the summary's page
	// count (minimal.pdf has 1 page, multipage.pdf 3) shows which file was
	// picked.
	a.waitFor("the diff view with multipage.pdf on the right", func() bool {
		q, err := a.query(`[data-testid="diff-summary"]`)
		return err == nil && len(q.Elements) > 0 && strings.Contains(q.Elements[0].Text, "Page count: 1 -> 3")
	})
}

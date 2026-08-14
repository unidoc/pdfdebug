// Package design_system_test provides acceptance tests for Design System
// Foundation and Theme Setup.
//
// These tests verify that CSS custom properties, font configuration,
// Tailwind v4 theme extension, and accessibility features are configured
// correctly in frontend/src/style.css.
//
// Test Levels: Integration (Go) -- CSS file content parsing.
// No browser interaction required; all criteria are CSS/theme validation.
//
// Run: go test ./tests/design-system/... -v -count=1
package design_system_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// projectRoot returns the absolute path to the project root directory.
// It walks upward from the test file location to find the project root,
// identified by the presence of a go.mod whose module name is "unidoc-pdf-debugger".
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if content, err := os.ReadFile(goModPath); err == nil {
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

// readStyleCSS reads frontend/src/style.css and returns its content.
func readStyleCSS(t *testing.T) string {
	t.Helper()
	root := projectRoot(t)
	stylePath := filepath.Join(root, "frontend", "src", "style.css")
	content, err := os.ReadFile(stylePath)
	if err != nil {
		t.Fatalf("failed to read frontend/src/style.css: %v", err)
	}
	return string(content)
}

// ---------------------------------------------------------------------------
// CSS custom properties for design tokens defined on :root: --color-bg,
// --color-surface, --color-surface-hover,
//       --color-surface-selected, --color-text, --color-text-secondary,
//       --color-text-muted, --color-border, --color-border-focus,
//       --color-tree-selected, --color-tree-hover, --font-ui, --font-mono,
//       --panel-padding, --tree-indent
// ---------------------------------------------------------------------------

func TestCSSCustomPropertiesDefinedOnRoot(t *testing.T) {
	css := readStyleCSS(t)

	// Extract the :root block(s) from the CSS
	rootBlockRe := regexp.MustCompile(`(?s):root\s*\{[^}]+\}`)
	rootBlocks := rootBlockRe.FindAllString(css, -1)
	if len(rootBlocks) == 0 {
		t.Fatal("no:root block found in style.css")
	}

	rootContent := strings.Join(rootBlocks, "\n")

	// Also extract @theme blocks -- Tailwind v4's @theme generates :root-level
	// CSS custom properties at build time, so font tokens defined in @theme
	// are effectively on :root in the compiled output.
	themeBlockRe := regexp.MustCompile(`(?s)@theme\s*\{[^}]+\}`)
	themeBlocks := themeBlockRe.FindAllString(css, -1)
	rootOrThemeContent := rootContent + "\n" + strings.Join(themeBlocks, "\n")

	// All design tokens that must be defined on :root Tokens with
	// checkTheme=true may be defined in @theme instead of :root (Tailwind
	// v4 @theme generates :root-level CSS vars at build time)
	type tokenCheck struct {
		property   string
		value      string // expected hex or value (empty = just check existence)
		checkTheme bool   // if true, also accept definition in @theme block
	}
	requiredTokens := []tokenCheck{
		{"--color-bg", "#f8fafc", false},
		{"--color-surface", "#ffffff", false},
		{"--color-surface-hover", "#f1f5f9", false},
		{"--color-surface-selected", "#eff6ff", false},
		{"--color-text", "#0f172a", false},
		{"--color-text-secondary", "#64748b", false},
		{"--color-text-muted", "#94a3b8", false},
		{"--color-border", "#e2e8f0", false},
		{"--color-border-focus", "#3b82f6", false},
		{"--color-tree-selected", "#eff6ff", false},
		{"--color-tree-hover", "#f1f5f9", false},
		{"--font-ui", "", true},
		{"--font-mono", "", true},
		{"--panel-padding", "12px", false},
		{"--tree-indent", "16px", false},
	}

	for _, token := range requiredTokens {
		// Use property + colon pattern to avoid substring false positives
		// (e.g., --color-text matching --color-text-secondary)
		searchContent := rootContent
		if token.checkTheme {
			searchContent = rootOrThemeContent
		}
		propColonRe := regexp.MustCompile(regexp.QuoteMeta(token.property) + `\s*:`)
		if !propColonRe.MatchString(searchContent) {
			location := ":root"
			if token.checkTheme {
				location = ":root or @theme"
			}
			t.Errorf("%s missing CSS custom property: %s", location, token.property)
			continue
		}
		if token.value != "" {
			// Check the property has the expected value
			escapedProp := regexp.QuoteMeta(token.property)
			escapedVal := regexp.QuoteMeta(token.value)
			valueRe := regexp.MustCompile(escapedProp + `\s*:\s*` + escapedVal)
			if !valueRe.MatchString(searchContent) {
				t.Errorf("root property %s does not have expected value %s", token.property, token.value)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// PDF value type color custom properties defined: --color-type-name
// (teal), --color-type-string (amber),
//       --color-type-number (blue), --color-type-reference (violet),
//       --color-type-boolean (pink), --color-type-null (gray),
//       --color-type-stream (emerald)
//       AND semantic colors: --color-error (red), --color-warning (amber),
//       --color-success (green), --color-info (blue)
// ---------------------------------------------------------------------------

func TestPDFTypeAndSemanticColorsDefined(t *testing.T) {
	css := readStyleCSS(t)

	rootBlockRe := regexp.MustCompile(`(?s):root\s*\{[^}]+\}`)
	rootBlocks := rootBlockRe.FindAllString(css, -1)
	if len(rootBlocks) == 0 {
		t.Fatal("no:root block found in style.css")
	}

	rootContent := strings.Join(rootBlocks, "\n")

	// PDF value type colors (first part)
	pdfTypeTokens := []struct {
		property string
		value    string
	}{
		{"--color-type-name", "#0d9488"},
		{"--color-type-string", "#d97706"},
		{"--color-type-number", "#2563eb"},
		{"--color-type-reference", "#7c3aed"},
		{"--color-type-boolean", "#db2777"},
		{"--color-type-null", "#94a3b8"},
		{"--color-type-stream", "#059669"},
	}

	for _, token := range pdfTypeTokens {
		escapedProp := regexp.QuoteMeta(token.property)
		escapedVal := regexp.QuoteMeta(token.value)
		valueRe := regexp.MustCompile(escapedProp + `\s*:\s*` + escapedVal)
		if !valueRe.MatchString(rootContent) {
			t.Errorf("root missing or incorrect PDF type color: %s expected %s", token.property, token.value)
		}
	}

	// Semantic colors (second part)
	semanticTokens := []struct {
		property string
		value    string
	}{
		{"--color-error", "#ef4444"},
		{"--color-warning", "#f59e0b"},
		{"--color-success", "#22c55e"},
		{"--color-info", "#3b82f6"},
	}

	for _, token := range semanticTokens {
		escapedProp := regexp.QuoteMeta(token.property)
		escapedVal := regexp.QuoteMeta(token.value)
		valueRe := regexp.MustCompile(escapedProp + `\s*:\s*` + escapedVal)
		if !valueRe.MatchString(rootContent) {
			t.Errorf("root missing or incorrect semantic color: %s expected %s", token.property, token.value)
		}
	}
}

// ---------------------------------------------------------------------------
// Inter font configured as UI font with system-ui fallback: Light theme
// defaults applied (body uses --color-bg and --color-text): Inter font
// configured with system-ui fallback
// ---------------------------------------------------------------------------

func TestInterFontAndBodyDefaults(t *testing.T) {
	css := readStyleCSS(t)

	// Verify font-ui token includes Inter with system-ui fallback Check
	// :root or @theme for --font-ui definition
	interFontRe := regexp.MustCompile(`--font-ui\s*:\s*['"]?Inter['"]?\s*,\s*system-ui`)
	if !interFontRe.MatchString(css) {
		t.Error("--font-ui not configured with Inter and system-ui fallback")
	}

	// Verify body uses design token variables for light theme defaults
	bodyBlockRe := regexp.MustCompile(`(?s)body\s*\{[^}]+\}`)
	bodyBlocks := bodyBlockRe.FindAllString(css, -1)
	if len(bodyBlocks) == 0 {
		t.Fatal("no body style block found in style.css")
	}

	bodyContent := strings.Join(bodyBlocks, "\n")

	// body must use font-family: var(--font-ui)
	if !strings.Contains(bodyContent, "font-family") || !strings.Contains(bodyContent, "var(--font-ui)") {
		t.Error("body does not set font-family: var(--font-ui)")
	}

	// body must set background-color: var(--color-bg)
	if !strings.Contains(bodyContent, "background-color") || !strings.Contains(bodyContent, "var(--color-bg)") {
		t.Error("body does not set background-color: var(--color-bg)")
	}

	// body must set color: var(--color-text)
	// Note: the precise regex check below supersedes this, but keep the guard correct.
	if !strings.Contains(bodyContent, "color") || !strings.Contains(bodyContent, "var(--color-text)") {
		t.Error("body does not set color: var(--color-text)")
	}

	// More precise check: color: var(--color-text) must appear
	colorTextRe := regexp.MustCompile(`(?m)^\s*color\s*:\s*var\(--color-text\)`)
	if !colorTextRe.MatchString(bodyContent) {
		t.Error("body block missing 'color: var(--color-text)' declaration")
	}
}

// ---------------------------------------------------------------------------
// JetBrains Mono configured as mono font: JetBrains Mono
// with ui-monospace fallback
// ---------------------------------------------------------------------------

func TestJetBrainsMonoFontConfigured(t *testing.T) {
	css := readStyleCSS(t)

	// Verify --font-mono includes JetBrains Mono with ui-monospace fallback
	monoFontRe := regexp.MustCompile(`--font-mono\s*:\s*['"]?JetBrains Mono['"]?\s*,\s*ui-monospace`)
	if !monoFontRe.MatchString(css) {
		t.Error("--font-mono not configured with JetBrains Mono and ui-monospace fallback")
	}
}

// ---------------------------------------------------------------------------
// prefers-reduced-motion disables CSS transitions: All CSS
// transitions and animations disabled when OS-level
//       prefers-reduced-motion is enabled
// ---------------------------------------------------------------------------

func TestReducedMotionSupport(t *testing.T) {
	css := readStyleCSS(t)

	// Verify @media (prefers-reduced-motion: reduce) block exists
	if !strings.Contains(css, "prefers-reduced-motion") {
		t.Fatal("no prefers-reduced-motion media query found in style.css")
	}

	// Extract the reduced-motion block
	reducedMotionRe := regexp.MustCompile(`(?s)@media\s*\(\s*prefers-reduced-motion\s*:\s*reduce\s*\)\s*\{.+?\}[\s]*\}`)
	match := reducedMotionRe.FindString(css)
	if match == "" {
		t.Fatal("@media (prefers-reduced-motion: reduce) block not found or malformed")
	}

	// Verify the block disables animations and transitions
	requiredDeclarations := []string{
		"animation-duration",
		"animation-iteration-count",
		"transition-duration",
		"scroll-behavior",
	}

	for _, decl := range requiredDeclarations {
		if !strings.Contains(match, decl) {
			t.Errorf("prefers-reduced-motion block missing declaration: %s", decl)
		}
	}

	// Verify !important is used (to override any inline or specific styles)
	if !strings.Contains(match, "!important") {
		t.Error("prefers-reduced-motion declarations should use !important")
	}
}

// ---------------------------------------------------------------------------
// Tailwind v4 @theme directive extends theme: Tailwind config extends
// default theme with custom spacing and fonts
// ---------------------------------------------------------------------------

func TestTailwindV4ThemeDirective(t *testing.T) {
	css := readStyleCSS(t)

	// Verify @theme directive exists (Tailwind v4 CSS-based config)
	if !strings.Contains(css, "@theme") {
		t.Fatal("no @theme directive found in style.css -- Tailwind v4 requires @theme for theme customization")
	}

	// Verify @theme block contains font registration
	themeBlockRe := regexp.MustCompile(`(?s)@theme\s*\{[^}]+\}`)
	themeBlocks := themeBlockRe.FindAllString(css, -1)
	if len(themeBlocks) == 0 {
		t.Fatal("no @theme {... } block found in style.css")
	}

	themeContent := strings.Join(themeBlocks, "\n")

	// Font families must be registered in @theme for utility class generation
	if !strings.Contains(themeContent, "--font-ui") {
		t.Error("@theme block missing --font-ui registration")
	}
	if !strings.Contains(themeContent, "--font-mono") {
		t.Error("@theme block missing --font-mono registration")
	}

	// Custom spacing scale (4px base) must be registered in @theme
	spacingTokens := []struct {
		property string
		value    string
	}{
		{"--spacing-1", "4px"},
		{"--spacing-2", "8px"},
		{"--spacing-3", "12px"},
		{"--spacing-4", "16px"},
		{"--spacing-6", "24px"},
		{"--spacing-8", "32px"},
	}

	for _, token := range spacingTokens {
		escapedProp := regexp.QuoteMeta(token.property)
		escapedVal := regexp.QuoteMeta(token.value)
		spacingRe := regexp.MustCompile(escapedProp + `\s*:\s*` + escapedVal)
		if !spacingRe.MatchString(themeContent) {
			t.Errorf("@theme block missing or incorrect spacing token: %s expected %s", token.property, token.value)
		}
	}

	// Custom type scale must be registered in @theme
	typeScaleTokens := []struct {
		property string
		value    string
	}{
		{"--text-xs", "11px"},
		{"--text-sm", "13px"},
		{"--text-base", "14px"},
		{"--text-lg", "16px"},
		{"--text-xl", "20px"},
	}

	for _, token := range typeScaleTokens {
		escapedProp := regexp.QuoteMeta(token.property)
		escapedVal := regexp.QuoteMeta(token.value)
		typeRe := regexp.MustCompile(escapedProp + `\s*:\s*` + escapedVal)
		if !typeRe.MatchString(themeContent) {
			t.Errorf("@theme block missing or incorrect type scale: %s expected %s", token.property, token.value)
		}
	}

	// Line heights for type scale
	lineHeightTokens := []struct {
		property string
		value    string
	}{
		{"--text-xs--line-height", "16px"},
		{"--text-sm--line-height", "20px"},
		{"--text-base--line-height", "22px"},
		{"--text-lg--line-height", "24px"},
		{"--text-xl--line-height", "28px"},
	}

	for _, token := range lineHeightTokens {
		escapedProp := regexp.QuoteMeta(token.property)
		escapedVal := regexp.QuoteMeta(token.value)
		lhRe := regexp.MustCompile(escapedProp + `\s*:\s*` + escapedVal)
		if !lhRe.MatchString(themeContent) {
			t.Errorf("@theme block missing or incorrect line height: %s expected %s", token.property, token.value)
		}
	}
}

// ---------------------------------------------------------------------------
// @theme inline registers color tokens for Tailwind utilities (part): Colors
// registered via @theme inline so var() references work
// ---------------------------------------------------------------------------

func TestTailwindThemeInlineColors(t *testing.T) {
	css := readStyleCSS(t)

	// Verify @theme inline directive exists
	if !strings.Contains(css, "@theme inline") {
		t.Fatal("no @theme inline directive found in style.css -- required for color token registration")
	}

	// Extract @theme inline block
	themeInlineRe := regexp.MustCompile(`(?s)@theme\s+inline\s*\{[^}]+\}`)
	inlineBlocks := themeInlineRe.FindAllString(css, -1)
	if len(inlineBlocks) == 0 {
		t.Fatal("no @theme inline {... } block found in style.css")
	}

	inlineContent := strings.Join(inlineBlocks, "\n")

	// All color tokens that must be registered in @theme inline
	colorTokens := []string{
		"--color-bg",
		"--color-surface",
		"--color-surface-hover",
		"--color-surface-selected",
		"--color-text",
		"--color-text-secondary",
		"--color-text-muted",
		"--color-border",
		"--color-border-focus",
		"--color-tree-selected",
		"--color-tree-hover",
		"--color-type-name",
		"--color-type-string",
		"--color-type-number",
		"--color-type-reference",
		"--color-type-boolean",
		"--color-type-null",
		"--color-type-stream",
		"--color-error",
		"--color-warning",
		"--color-success",
		"--color-info",
	}

	for _, token := range colorTokens {
		if !strings.Contains(inlineContent, token) {
			t.Errorf("@theme inline block missing color token: %s", token)
		}
	}
}

// ---------------------------------------------------------------------------
// Wails template Inter-Medium.ttf deleted Story task 2.5:
// Delete frontend/public/Inter-Medium.ttf
// ---------------------------------------------------------------------------

func TestWailsTemplateFontDeleted(t *testing.T) {
	root := projectRoot(t)
	fontPath := filepath.Join(root, "frontend", "public", "Inter-Medium.ttf")

	if _, err := os.Stat(fontPath); err == nil {
		t.Error("frontend/public/Inter-Medium.ttf still exists -- it is replaced by the Google Fonts CDN import and must be deleted")
	} else if !os.IsNotExist(err) {
		t.Errorf("unexpected error checking font file: %v", err)
	}
	// If os.Stat returns os.ErrNotExist, test passes (file correctly deleted)
}

// ---------------------------------------------------------------------------
// style.css retains its Tailwind import -- regression guard.
// ---------------------------------------------------------------------------

func TestTailwindImportRetained(t *testing.T) {
	css := readStyleCSS(t)

	if !strings.Contains(css, `@import "tailwindcss"`) && !strings.Contains(css, `@import 'tailwindcss'`) {
		t.Fatal("style.css missing @import \"tailwindcss\" -- Tailwind CSS must remain imported")
	}
}

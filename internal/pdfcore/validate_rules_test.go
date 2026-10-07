package pdfcore

import (
	"strings"
	"testing"
)

// registryFor returns the registry entries of profile, in registry order.
func registryFor(profile string) []rule {
	var out []rule
	for _, r := range ruleRegistry {
		if r.profile == profile {
			out = append(out, r)
		}
	}
	return out
}

// verdictPhrases are authoritative conformance verdicts no rule phrase or scope
// sentence may contain (case-insensitive substrings).
var verdictPhrases = []string{
	"pdf/a compliant", "pdf/a-compliant", "pdf/ua compliant", "is compliant",
	"fully compliant", "conformant", "is valid", "valid pdf/a", "pdf/a valid",
	"passed validation", "validation passed", "compliance: pass",
}

func assertHonestText(t *testing.T, label, s string) {
	t.Helper()
	low := strings.ToLower(s)
	for _, p := range verdictPhrases {
		if strings.Contains(low, p) {
			t.Errorf("%s contains verdict %q: %q", label, p, s)
		}
	}
	for i := 0; i < len(s); i++ {
		if s[i] > 0x7f {
			t.Errorf("%s contains non-ASCII byte 0x%02x: %q", label, s[i], s)
			return
		}
	}
}

func TestProfileRules_ListsRegistryRulesInOrder(t *testing.T) {
	for _, profile := range ValidProfiles {
		got := ProfileRules(profile)
		want := registryFor(profile)
		if len(got) != len(want) {
			t.Fatalf("%s: ProfileRules returned %d rules, registry has %d", profile, len(got), len(want))
		}
		for i, r := range want {
			g := got[i]
			if g.RuleID != r.id || g.SpecRef != r.specRef || g.Severity != r.severity {
				t.Errorf("%s rule %d = %+v, want id %q specRef %q severity %q", profile, i, g, r.id, r.specRef, r.severity)
			}
			if strings.TrimSpace(g.Checks) == "" || g.SpecRef == "" {
				t.Errorf("%s rule %q must carry non-empty Checks and SpecRef: %+v", profile, g.RuleID, g)
			}
			if g.Evaluated || g.Findings != 0 {
				t.Errorf("%s rule %q from ProfileRules must have zero Evaluated/Findings: %+v", profile, g.RuleID, g)
			}
			assertHonestText(t, "checks for "+g.RuleID, g.Checks)
		}
	}
}

// Every surface prints "none of the N rules checked found a problem" from the
// rule count, so a profile with no rules would print a clean line over zero
// checks; the GUI keys rule rows by id, so ids must be unique within a profile.
func TestProfileRules_EveryProfileHasDistinctRules(t *testing.T) {
	for _, profile := range ValidProfiles {
		rules := ProfileRules(profile)
		if len(rules) == 0 {
			t.Errorf("profile %q has no registry rules", profile)
		}
		seen := map[string]bool{}
		for _, r := range rules {
			if seen[r.RuleID] {
				t.Errorf("profile %q lists rule %q twice", profile, r.RuleID)
			}
			seen[r.RuleID] = true
		}
	}
}

func TestProfileScopeAndStandard_NonEmptyPerProfile(t *testing.T) {
	scopes := map[string]bool{}
	for _, profile := range ValidProfiles {
		scope := ProfileScope(profile)
		if strings.TrimSpace(scope) == "" {
			t.Errorf("ProfileScope(%q) is empty", profile)
		}
		if ProfileStandard(profile) == "" {
			t.Errorf("ProfileStandard(%q) is empty", profile)
		}
		if !strings.Contains(strings.ToLower(scope), "subset") {
			t.Errorf("ProfileScope(%q) must say the profile is a subset: %q", profile, scope)
		}
		if scopes[scope] {
			t.Errorf("ProfileScope(%q) repeats another profile's scope", profile)
		}
		scopes[scope] = true
		assertHonestText(t, "scope for "+profile, scope)
	}
	if got := ProfileStandard(ProfilePDFA1B); got != "PDF/A-1b" {
		t.Errorf("ProfileStandard(pdfa-1b) = %q, want PDF/A-1b", got)
	}
	if got := ProfileStandard(ProfilePDFUA1Structural); got != "PDF/UA-1" {
		t.Errorf("ProfileStandard(pdfua-1-structural) = %q, want PDF/UA-1", got)
	}
	ua := strings.ToLower(ProfileScope(ProfilePDFUA1Structural))
	for _, want := range []string{"pdf/ua-1", "not a pdf/ua-1 conformance check", "marked content", "structure tree", "alternate text", "font"} {
		if !strings.Contains(ua, want) {
			t.Errorf("pdfua-1-structural scope must mention %q: %q", want, ua)
		}
	}
}

func TestProfileHelpers_UnknownProfile(t *testing.T) {
	rules := ProfileRules("no-such-profile")
	if rules == nil || len(rules) != 0 {
		t.Errorf("ProfileRules(unknown) = %#v, want an empty non-nil slice", rules)
	}
	if s := ProfileScope("no-such-profile"); s != "" {
		t.Errorf("ProfileScope(unknown) = %q, want empty", s)
	}
	if s := ProfileStandard("no-such-profile"); s != "" {
		t.Errorf("ProfileStandard(unknown) = %q, want empty", s)
	}
}

// assertRulesMatchRun checks that a result lists every profile rule with
// findings equal to that rule's problem count.
func assertRulesMatchRun(t *testing.T, res *ValidationResult, profile string) {
	t.Helper()
	if res.Scope != ProfileScope(profile) {
		t.Errorf("Scope = %q, want ProfileScope(%q) = %q", res.Scope, profile, ProfileScope(profile))
	}
	want := ProfileRules(profile)
	if len(res.Rules) != len(want) {
		t.Fatalf("Rules has %d entries, ProfileRules(%q) has %d", len(res.Rules), profile, len(want))
	}
	for i, w := range want {
		g := res.Rules[i]
		if g.RuleID != w.RuleID || g.SpecRef != w.SpecRef || g.Severity != w.Severity || g.Checks != w.Checks {
			t.Errorf("Rules[%d] = %+v, want the ProfileRules entry %+v", i, g, w)
		}
		if !g.Evaluated {
			t.Errorf("rule %q ran but Evaluated=false", g.RuleID)
		}
		if n := countByRule(res.Problems, g.RuleID); g.Findings != n {
			t.Errorf("rule %q Findings = %d, problems = %d", g.RuleID, g.Findings, n)
		}
	}
}

func TestValidate_RulesAndScopeOnUntaggedDocument(t *testing.T) {
	ins, tabID := writeTempPDF(t, "untagged.pdf", assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n",
	))
	for _, profile := range ValidProfiles {
		res, err := ins.Validate(tabID, profile)
		if err != nil {
			t.Fatalf("Validate(%s): %v", profile, err)
		}
		assertRulesMatchRun(t, res, profile)
	}
	res, err := ins.Validate(tabID, ProfilePDFUA1Structural)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	for _, r := range res.Rules {
		if r.Findings != 1 {
			t.Errorf("untagged doc: rule %q Findings = %d, want 1", r.RuleID, r.Findings)
		}
	}
}

func TestValidate_RulesAndScopeOnTaggedDocument(t *testing.T) {
	ins, tabID := writeTempPDF(t, "tagged.pdf", assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 4 0 R /Lang (en-US) >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n",
		"4 0 obj\n<< /Type /StructTreeRoot >>\nendobj\n\n",
	))
	res, err := ins.Validate(tabID, ProfilePDFUA1Structural)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	assertRulesMatchRun(t, res, ProfilePDFUA1Structural)
	for _, r := range res.Rules {
		if r.Findings != 0 {
			t.Errorf("tagged doc: rule %q Findings = %d, want 0", r.RuleID, r.Findings)
		}
	}
}

// Mutates the package-level registry, so it must not run in parallel.
func TestValidate_DegradedRuleIsNotEvaluated(t *testing.T) {
	saved := ruleRegistry
	t.Cleanup(func() { ruleRegistry = saved })
	ruleRegistry = []rule{
		{
			id: "always-panics", profile: ProfilePDFUA1Structural, severity: "warning",
			specRef: "ISO 14289-1:2014, 7.1",
			check:   func(*DocumentState) []ruleHit { panic("check exploded") },
		},
		{
			id: "lang", profile: ProfilePDFUA1Structural, severity: "warning",
			specRef: "ISO 14289-1:2014, 7.2", check: checkLang,
		},
	}

	ins, tabID := writeTempPDF(t, "untagged.pdf", assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n",
	))
	res, err := ins.Validate(tabID, ProfilePDFUA1Structural)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(res.Rules) != 2 {
		t.Fatalf("Rules = %+v, want both registry rules", res.Rules)
	}
	degraded := res.Rules[0]
	if degraded.RuleID != "always-panics" || degraded.Evaluated || degraded.Findings != 0 {
		t.Errorf("degraded rule entry = %+v, want Evaluated=false Findings=0", degraded)
	}
	if p := problemByRule(res.Problems, "always-panics"); p == nil || p.Severity != "info" {
		t.Errorf("degraded rule must still emit one info problem, got %+v", p)
	}
	lang := res.Rules[1]
	if lang.RuleID != "lang" || !lang.Evaluated || lang.Findings != 1 {
		t.Errorf("lang entry = %+v, want Evaluated=true Findings=1", lang)
	}
}

func TestEncryptedResult_ListsOnlyNoEncryption(t *testing.T) {
	res := EncryptedResult(ProfilePDFA1B)
	if len(res.Rules) != 1 {
		t.Fatalf("EncryptedResult Rules = %+v, want only no-encryption", res.Rules)
	}
	var want RuleInfo
	for _, r := range ProfileRules(ProfilePDFA1B) {
		if r.RuleID == "no-encryption" {
			want = r
		}
	}
	if want.RuleID == "" {
		t.Fatalf("ProfileRules(pdfa-1b) has no no-encryption entry")
	}
	got := res.Rules[0]
	if !got.Evaluated || got.Findings != 1 {
		t.Errorf("no-encryption entry = %+v, want Evaluated=true Findings=1", got)
	}
	got.Evaluated, got.Findings = want.Evaluated, want.Findings
	if got != want {
		t.Errorf("no-encryption entry %+v differs from the ProfileRules entry %+v", got, want)
	}
	scope := ProfileScope(ProfilePDFA1B)
	if scope == "" || !strings.HasPrefix(res.Scope, scope) || len(res.Scope) <= len(scope) {
		t.Errorf("EncryptedResult Scope = %q, want ProfileScope(pdfa-1b) %q plus a clause", res.Scope, scope)
	}
	assertHonestText(t, "encrypted scope", res.Scope)
	if res.Summary.Errors != 1 || len(res.Problems) != 1 || res.Disclaimer != DisclaimerText {
		t.Errorf("EncryptedResult existing fields changed: %+v", res)
	}
}

// A non-embedded font written inline in /Resources is not visited, and a device
// colour space named as a cs operand triggers the OutputIntent requirement; the
// check phrases must state both so the printed outcome matches the phrase.
func TestValidate_ChecksPhrasesMatchFontAndDeviceColourReach(t *testing.T) {
	ins, tabID := writeTempPDF(t, "inline-font.pdf", assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> >> >> /Contents 4 0 R >>\nendobj\n\n",
		"4 0 obj\n<< /Length 30 >>\nstream\n/DeviceRGB cs 1 0 0 sc 0 0 m\nendstream\nendobj\n\n",
	))
	res, err := ins.Validate(tabID, ProfilePDFA1B)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	byID := map[string]RuleInfo{}
	for _, r := range res.Rules {
		byID[r.RuleID] = r
	}
	font := byID["font-embedding"]
	if font.Findings == 0 && (!strings.Contains(font.Checks, "indirect") || !strings.Contains(font.Checks, "inline")) {
		t.Errorf("font-embedding found nothing on an inline non-embedded font, so its phrase must say inline fonts are not read: %q", font.Checks)
	}
	oi := byID["output-intent"]
	if oi.Findings != 1 {
		t.Errorf("output-intent Findings = %d on a /DeviceRGB cs stream with no /OutputIntents, want 1", oi.Findings)
	}
	if !strings.Contains(oi.Checks, "/DeviceRGB") {
		t.Errorf("output-intent fires on a /DeviceRGB cs operand, so its phrase must name it: %q", oi.Checks)
	}
}

func TestValidate_NoJSLaunchPhraseNamesInlineActionReach(t *testing.T) {
	ins, tabID := writeTempPDF(t, "inline-action.pdf", assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [4 0 R] >>\nendobj\n\n",
		"4 0 obj\n<< /Type /Annot /Subtype /Link /Rect [0 0 10 10] /A << /S /JavaScript /JS (app.alert(1)) >> >>\nendobj\n\n",
	))
	res, err := ins.Validate(tabID, ProfilePDFA1B)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	for _, r := range res.Rules {
		if r.RuleID != "no-js-launch" {
			continue
		}
		if r.Findings == 0 && !strings.Contains(r.Checks, "inline") {
			t.Errorf("no-js-launch found nothing on an inline annotation action, so its phrase must say inline actions are not read: %q", r.Checks)
		}
		return
	}
	t.Fatal("no-js-launch missing from Rules")
}

func TestValidate_EmptyProfileFillsPDFARulesAndScope(t *testing.T) {
	ins, tabID := writeTempPDF(t, "untagged.pdf", assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n",
	))
	res, err := ins.Validate(tabID, "")
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Profile != ProfilePDFA1B {
		t.Fatalf("Profile = %q, want %q", res.Profile, ProfilePDFA1B)
	}
	assertRulesMatchRun(t, res, ProfilePDFA1B)
}

func TestEncryptedResult_EmptyProfileDefaultsToPDFA(t *testing.T) {
	res := EncryptedResult("")
	if res.Profile != ProfilePDFA1B {
		t.Errorf("Profile = %q, want %q", res.Profile, ProfilePDFA1B)
	}
	if !strings.HasPrefix(res.Scope, ProfileScope(ProfilePDFA1B)+" ") {
		t.Errorf("Scope = %q, want the pdfa-1b scope plus a clause", res.Scope)
	}
	if len(res.Rules) != 1 || res.Rules[0].RuleID != "no-encryption" {
		t.Errorf("Rules = %+v, want only no-encryption", res.Rules)
	}
}

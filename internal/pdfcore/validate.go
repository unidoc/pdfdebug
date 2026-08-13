package pdfcore

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Severity classifies a Problem. It is a fixed property declared per rule in
// the registry (never computed per run): PDF/A-1b structural rules are always
// "error" and gate the CLI exit code; PDF/UA-1 structural rules are always
// "warning" and never gate. "info" is reserved for the degraded-rule signal.
type Severity = string

// Profile names the bounded, versioned rule subset selected for a run. Exactly
// one profile runs per Validate call.
const (
	// ProfilePDFA1B is the default profile: the PDF/A-1b structural rule set
	// (all error severity).
	ProfilePDFA1B = "pdfa-1b"
	// ProfilePDFUA1Structural is the PDF/UA-1 structural subset (all warning
	// severity, non-gating).
	ProfilePDFUA1Structural = "pdfua-1-structural"
)

// ValidProfiles lists every accepted --profile value, in display order. Used by
// the CLI to report an unknown profile and by callers to enumerate choices.
var ValidProfiles = []string{ProfilePDFA1B, ProfilePDFUA1Structural}

// DisclaimerText is the always-visible honesty guardrail. It states
// plainly that the checks are structural only and defers authoritative
// conformance to veraPDF. It is carried on every ValidationResult so JSON
// consumers never have to infer it.
const DisclaimerText = "structural checks only - not full conformance; use veraPDF for authoritative PDF/A / PDF/UA validation"

// ErrUnknownProfile signals an unrecognized --profile value. The CLI maps it to
// the operational-error exit code (2) with the valid profiles listed to stderr.
var ErrUnknownProfile = errors.New("unknown validation profile")

// Problem is one structural conformance finding. Severity/RuleID/Profile/SpecRef
// come from the rule registry (the single source of truth); Message/ObjRef/
// ObjNodeID are computed by the rule's check. ObjNodeID is the obj:{gen}:{num}
// tree-node id (gen-then-num) the GUI selection wiring consumes; it is present
// whenever ObjRef is. Document-level problems (e.g. missing /Lang) carry empty
// ObjRef/ObjNodeID.
type Problem struct {
	// RuleID is the stable rule identifier (e.g. "font-embedding").
	RuleID string `json:"ruleId"`
	// Profile is the profile that emitted this problem (self-describing for
	// JSON consumers and future multi-profile runs).
	Profile string `json:"profile"`
	// Severity is the registry-declared severity ("error" | "warning" | "info").
	Severity Severity `json:"severity"`
	// Message is the human-readable defect description.
	Message string `json:"message"`
	// ObjRef is the "N G R" reference of the offending object; empty for
	// document-level problems.
	ObjRef string `json:"objRef"`
	// ObjNodeID is the obj:{gen}:{num} tree-node id; present whenever ObjRef is.
	ObjNodeID string `json:"objNodeId"`
	// SpecRef is the ISO 32000 / PDF/A / PDF/UA clause reference (never empty).
	SpecRef string `json:"specRef"`
}

// ValidationSummary is the error/warning/info tally over all problems.
type ValidationSummary struct {
	// Errors is the count of error-severity problems (the CI gate signal).
	Errors int `json:"errors"`
	// Warnings is the count of warning-severity problems.
	Warnings int `json:"warnings"`
	// Info is the count of info-severity problems (degraded rules that could
	// not be evaluated). Surfaced so a "0 errors, 0 warnings" summary never
	// silently hides a rule that failed to run.
	Info int `json:"info"`
}

// ValidationResult is the document-level outcome of one Validate run: the
// selected profile, the tally, every problem, and the always-present honesty
// disclaimer. It is the shape the CLI marshals and the GUI binding returns.
type ValidationResult struct {
	// Profile is the profile that ran.
	Profile string `json:"profile"`
	// Summary is the error/warning tally.
	Summary ValidationSummary `json:"summary"`
	// Problems is every finding, rule order. Non-nil (empty on a clean run).
	Problems []Problem `json:"problems"`
	// Disclaimer is the not-authoritative note, always populated.
	Disclaimer string `json:"disclaimer"`
}

// ruleHit is a rule's raw finding before the engine stamps the registry-owned
// RuleID/Profile/Severity/SpecRef onto it.
type ruleHit struct {
	message   string
	objRef    string
	objNodeID string
}

// rule is one entry in the bounded, documented rule registry. Adding a rule is
// a code change, not a config change. Severity is a fixed property of the rule
// here, not computed per run.
type rule struct {
	id       string
	profile  string
	severity Severity
	specRef  string
	check    func(doc *DocumentState) []ruleHit
}

// ruleRegistry is the single source of truth for the bounded structural rule
// set. PDF/A-1b rules are all "error" (they gate the CLI exit code); PDF/UA-1
// structural rules are all "warning" (informational, never gating).
//
// RULE DELTA (veraPDF checks we deliberately do NOT implement - the structural
// firewall, cross-referenced from tests/compliance-validation/
// oracle_verapdf_test.go): transparency / blend-mode rules (PDF/A-1 6.2.4,
// 6.4), annotation-flag rules (6.5.3), action restrictions beyond JavaScript /
// Launch (6.6.1), the full XMP-property schema (6.7.x beyond packet presence +
// /Info consistency), interactive-form / NeedAppearances (6.9), and ICC-profile
// internal correctness (only /OutputIntent PRESENCE is ours, not profile
// validity). Those are veraPDF's job; this tool never claims authoritative
// conformance. See DisclaimerText.
var ruleRegistry = []rule{
	// --- PDF/A-1b structural rules (all error, gating) -----------------------
	{
		id: "font-embedding", profile: ProfilePDFA1B, severity: "error",
		specRef: "ISO 19005-1:2005, 6.3.4", check: checkFontEmbedding,
	},
	{
		id: "no-encryption", profile: ProfilePDFA1B, severity: "error",
		specRef: "ISO 19005-1:2005, 6.1.3", check: checkNoEncryption,
	},
	{
		id: "output-intent", profile: ProfilePDFA1B, severity: "error",
		specRef: "ISO 19005-1:2005, 6.2.2", check: checkOutputIntent,
	},
	{
		id: "no-js-launch", profile: ProfilePDFA1B, severity: "error",
		specRef: "ISO 19005-1:2005, 6.6.1", check: checkNoJSLaunch,
	},
	{
		id: "xmp-metadata", profile: ProfilePDFA1B, severity: "error",
		specRef: "ISO 19005-1:2005, 6.7.2/6.7.3", check: checkXMPMetadata,
	},
	{
		id: "document-id", profile: ProfilePDFA1B, severity: "error",
		specRef: "ISO 19005-1:2005, 6.1.3", check: checkDocumentID,
	},
	// --- PDF/UA-1 structural subset (all warning, non-gating) ----------------
	{
		id: "marked", profile: ProfilePDFUA1Structural, severity: "warning",
		specRef: "ISO 14289-1:2014, 7.1", check: checkMarked,
	},
	{
		id: "struct-tree-root", profile: ProfilePDFUA1Structural, severity: "warning",
		specRef: "ISO 14289-1:2014, 7.1", check: checkStructTreeRoot,
	},
	{
		id: "lang", profile: ProfilePDFUA1Structural, severity: "warning",
		specRef: "ISO 14289-1:2014, 7.2", check: checkLang,
	},
}

// ProfileGates reports whether a profile's rules gate the CLI exit code, i.e.
// the profile contains at least one error-severity rule (pdfa-1b does;
// pdfua-1-structural, all warnings, does not). Used by the CLI so a gating-rule
// that could not be evaluated (degraded to info) fails closed rather than
// reporting a clean exit 0.
func ProfileGates(profile string) bool {
	for _, r := range ruleRegistry {
		if r.profile == profile && r.severity == "error" {
			return true
		}
	}
	return false
}

// Validate runs the bounded rule set for the selected profile against the
// document in tabID and returns the problem list, tally, and disclaimer. An
// empty profile defaults to pdfa-1b; an unrecognized profile returns
// ErrUnknownProfile (the CLI maps it to the operational exit). Runs under
// doc.pdfMu; each rule is safeCall-wrapped so a rule that panics internally
// degrades to one info-severity problem rather than failing the whole run.
func (ins *Inspector) Validate(tabID, profile string) (*ValidationResult, error) {
	if profile == "" {
		profile = ProfilePDFA1B
	}
	if !slices.Contains(ValidProfiles, profile) {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProfile, profile)
	}

	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// Serialize pdfcpu access: every rule dereferences catalog/trailer/object
	// entries that touch pdfcpu's XRefTable.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	res := &ValidationResult{
		Profile:    profile,
		Problems:   []Problem{},
		Disclaimer: DisclaimerText,
	}
	for _, r := range ruleRegistry {
		if r.profile != profile {
			continue
		}
		hits, ruleErr := runRule(doc, r)
		if ruleErr != nil {
			// A rule that errors internally degrades to a single info
			// problem, never a whole-run failure. This catches ANY panic,
			// including a runtime.Error (nil deref, bad type assertion) that
			// safeCall deliberately re-panics -- a bug in one rule must not
			// abort the bounded run.
			res.Problems = append(res.Problems, Problem{
				RuleID:   r.id,
				Profile:  profile,
				Severity: "info",
				Message:  fmt.Sprintf("rule %q could not be evaluated: %v", r.id, ruleErr),
				SpecRef:  r.specRef,
			})
			continue
		}
		for _, h := range hits {
			res.Problems = append(res.Problems, Problem{
				RuleID:    r.id,
				Profile:   profile,
				Severity:  r.severity,
				Message:   h.message,
				ObjRef:    h.objRef,
				ObjNodeID: h.objNodeID,
				SpecRef:   r.specRef,
			})
		}
	}
	for _, p := range res.Problems {
		switch p.Severity {
		case "error":
			res.Summary.Errors++
		case "warning":
			res.Summary.Warnings++
		case "info":
			res.Summary.Info++
		}
	}
	return res, nil
}

// runRule runs one rule's check and returns its hits, converting ANY panic
// (including a runtime.Error that safeCall re-panics) into an error so the
// caller can degrade the rule to an info problem rather than crashing the whole
// run. This is a deliberate exception to the project-wide "runtime.Error
// surfaces loudly" rule: a read-only inspector over arbitrary PDFs must keep
// evaluating the remaining rules when one rule trips on malformed input.
func runRule(doc *DocumentState, r rule) (hits []ruleHit, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			hits = nil
			err = fmt.Errorf("%v", rec)
		}
	}()
	hits = r.check(doc)
	return hits, nil
}

// EncryptedResult builds a single-problem ValidationResult reporting that the
// document is encrypted, for the reconciliation path where the Inspector
// refused to open the file with ErrEncryptedPDF. PDF/A forbids encryption, so
// this is an error-severity problem (exit 1), not a bare operational open
// failure.
func EncryptedResult(profile string) *ValidationResult {
	if profile == "" {
		profile = ProfilePDFA1B
	}
	p := Problem{
		RuleID:   "no-encryption",
		Profile:  profile,
		Severity: "error",
		Message:  "document is encrypted (PDF/A forbids encryption)",
		SpecRef:  "ISO 19005-1:2005, 6.1.3",
	}
	return &ValidationResult{
		Profile:    profile,
		Summary:    ValidationSummary{Errors: 1},
		Problems:   []Problem{p},
		Disclaimer: DisclaimerText,
	}
}

// --- PDF/A-1b rule checks ----------------------------------------------------

// fontFileKeys are the FontDescriptor entries that carry an embedded font
// program (any one present means the font is embedded).
var fontFileKeys = []string{"FontFile", "FontFile2", "FontFile3"}

// checkFontEmbedding flags every /Type /Font whose font program is not embedded
// (PDF/A-1b 6.3.4 forbids non-embedded fonts). Type3 fonts define glyphs as
// content streams and carry no FontFile, so they are exempt. Type0 composite
// fonts are embedded via the descendant CIDFont's FontDescriptor.
func checkFontEmbedding(doc *DocumentState) []ruleHit {
	var hits []ruleHit
	forEachObject(doc, func(objNum, gen int, obj pdfcpu_types.Object) {
		d := asDict(obj)
		if d == nil || dictName(doc, d, "Type") != "Font" {
			return
		}
		subtype := dictName(doc, d, "Subtype")
		if subtype == "Type3" {
			return
		}
		// Descendant CIDFonts are /Type /Font too; they are evaluated via their
		// Type0 parent's DescendantFonts scan (fontIsEmbedded). Skip them here so
		// a non-embedded composite font is reported once (on the Type0), not
		// twice (Type0 + descendant).
		if subtype == "CIDFontType0" || subtype == "CIDFontType2" {
			return
		}
		if fontIsEmbedded(doc, d, subtype) {
			return
		}
		label := dictName(doc, d, "BaseFont")
		if label == "" {
			label = subtype
		}
		if label == "" {
			label = "(unnamed)"
		}
		hits = append(hits, ruleHit{
			message:   fmt.Sprintf("font /%s is not embedded", label),
			objRef:    fmt.Sprintf("%d %d R", objNum, gen),
			objNodeID: fmt.Sprintf("obj:%d:%d", gen, objNum),
		})
	})
	return hits
}

// fontIsEmbedded reports whether the font dict carries an embedded font program.
// Composite (Type0) fonts embed via the descendant CIDFont's FontDescriptor.
func fontIsEmbedded(doc *DocumentState, d pdfcpu_types.Dict, subtype string) bool {
	if subtype == "Type0" {
		for _, desc := range dereferenceArray(doc, d["DescendantFonts"]) {
			dd := asDict(dereference(doc, desc))
			if dd == nil {
				continue
			}
			if fd := asDict(dereference(doc, dd["FontDescriptor"])); fd != nil && hasFontFile(fd) {
				return true
			}
		}
		return false
	}
	fd := asDict(dereference(doc, d["FontDescriptor"]))
	return fd != nil && hasFontFile(fd)
}

// hasFontFile reports whether a FontDescriptor dict declares an embedded font
// program via any FontFile* entry.
func hasFontFile(fd pdfcpu_types.Dict) bool {
	for _, k := range fontFileKeys {
		if _, ok := fd[k]; ok {
			return true
		}
	}
	return false
}

// checkNoEncryption flags a document that carries an /Encrypt trailer entry
// (PDF/A-1b 6.1.3 forbids encryption). This covers openable-but-encrypted files
// (empty user password); a fully-unopenable ErrEncryptedPDF is reconciled by
// the caller via EncryptedResult.
func checkNoEncryption(doc *DocumentState) []ruleHit {
	xrt := doc.PDFContext.XRefTable
	if xrt == nil || xrt.Encrypt == nil {
		return nil
	}
	ref := *xrt.Encrypt
	return []ruleHit{{
		message:   "document is encrypted (/Encrypt present; PDF/A forbids encryption)",
		objRef:    refString(ref),
		objNodeID: nodeIDFromRef(ref),
	}}
}

// checkOutputIntent flags a document that uses device-dependent color but
// declares no /OutputIntents (PDF/A-1b 6.2.2). The rule is gated on actual
// device-color usage detected in page content streams so it does not fire on a
// document that never sets a device color (matching veraPDF, which does not
// require an OutputIntent when no device color space is used).
func checkOutputIntent(doc *DocumentState) []ruleHit {
	if catalogHasNonEmptyArray(doc, "OutputIntents") {
		return nil
	}
	if !documentUsesDeviceColor(doc) {
		return nil
	}
	return []ruleHit{{
		message: "document uses device color but declares no /OutputIntent (device-independent color / OutputIntent required)",
	}}
}

// deviceColorOps is the set of content-stream operators that set a
// device-dependent color (used by the OutputIntent device-color gate).
var deviceColorOps = map[string]bool{"rg": true, "RG": true, "k": true, "K": true, "g": true, "G": true}

// actionKind classifies a dict as a forbidden action, or "" if it is not one.
// An action is /S /JavaScript or /S /Launch; a dict carrying a /JS entry
// (JavaScript payload) is treated as JavaScript even when /S is absent.
func actionKind(doc *DocumentState, d pdfcpu_types.Dict) string {
	switch dictName(doc, d, "S") {
	case "JavaScript":
		return "JavaScript"
	case "Launch":
		return "Launch"
	}
	if _, ok := d["JS"]; ok {
		return "JavaScript"
	}
	return ""
}

// checkNoJSLaunch flags JavaScript actions and Launch actions in the object
// graph (PDF/A-1b 6.6.1 forbids both). An action dict is identified by
// /S /JavaScript, /S /Launch, or a /JS payload entry. Coverage is bounded to
// indirect objects plus the catalog's direct /OpenAction (the common inline
// case); actions reachable only via the /Names /JavaScript name tree or /AA
// additional-action dicts are not exhaustively walked (structural firewall - a
// missed action under-reports, never falsely flags).
func checkNoJSLaunch(doc *DocumentState) []ruleHit {
	var hits []ruleHit
	forEachObject(doc, func(objNum, gen int, obj pdfcpu_types.Object) {
		d := asDict(obj)
		if d == nil {
			return
		}
		if kind := actionKind(doc, d); kind != "" {
			hits = append(hits, ruleHit{
				message:   fmt.Sprintf("%s action present (forbidden in PDF/A)", kind),
				objRef:    fmt.Sprintf("%d %d R", objNum, gen),
				objNodeID: fmt.Sprintf("obj:%d:%d", gen, objNum),
			})
		}
	})
	// Catalog /OpenAction is frequently written as a direct (inline) action dict
	// that forEachObject (indirect objects only) never visits. Only handle the
	// DIRECT form here: an indirect /OpenAction points at an object forEachObject
	// already reported, so following it would double-count the same action.
	if cat := catalogDict(doc); cat != nil {
		if oaRaw := cat["OpenAction"]; oaRaw != nil {
			if _, isRef := oaRaw.(pdfcpu_types.IndirectRef); !isRef {
				if oa := asDict(oaRaw); oa != nil {
					if kind := actionKind(doc, oa); kind != "" {
						hits = append(hits, ruleHit{
							message: fmt.Sprintf("%s action present in catalog /OpenAction (forbidden in PDF/A)", kind),
						})
					}
				}
			}
		}
	}
	return hits
}

// infoXMPMap pairs each /Info key with the XMP property whose value must match
// it when both are present (PDF/A-1 6.7.3). Date fields (CreationDate/ModDate)
// are deliberately EXCLUDED: /Info stores PDF date syntax ("D:20240101120000Z")
// while XMP stores ISO-8601 ("2024-01-01T12:00:00Z"), so a byte comparison is a
// guaranteed false mismatch. Authoritative date-consistency is veraPDF's job
// (structural firewall); we compare only free-text fields.
var infoXMPMap = []struct {
	infoKey  string
	xmpProps []string
}{
	{"Title", []string{"dc:title"}},
	{"Author", []string{"dc:creator"}},
	{"Subject", []string{"dc:description"}},
	{"Keywords", []string{"pdf:Keywords"}},
	{"Creator", []string{"xmp:CreatorTool"}},
	{"Producer", []string{"pdf:Producer"}},
}

// wsRun matches a run of whitespace, collapsed to a single space before an
// /Info<->XMP comparison so a pretty-printed multi-line XMP value is not a false
// mismatch against the single-line /Info value.
var wsRun = regexp.MustCompile(`\s+`)

// normalizeXMPText collapses internal whitespace runs and trims, so semantically
// equal values that differ only in XML pretty-print whitespace compare equal.
func normalizeXMPText(s string) string {
	return strings.TrimSpace(wsRun.ReplaceAllString(s, " "))
}

// checkXMPMetadata flags a missing XMP metadata packet and any /Info<->XMP
// value inconsistency (PDF/A-1 6.7.2 presence, 6.7.3 consistency). The
// consistency check compares only keys present in BOTH /Info and the XMP
// packet; when a value cannot be extracted from the XMP it is skipped (never a
// false mismatch).
func checkXMPMetadata(doc *DocumentState) []ruleHit {
	xmp := catalogXMP(doc)
	if strings.TrimSpace(xmp) == "" {
		return []ruleHit{{message: "XMP metadata packet is missing (/Metadata)"}}
	}
	info := infoDict(doc)
	if info == nil {
		return nil
	}
	var hits []ruleHit
	for _, m := range infoXMPMap {
		// Decode the /Info value as a PDF text string (UTF-16BE with BOM, UTF-8,
		// or pdfcpu's Latin-1 byte fallback -- see decodeTextString) so a
		// non-ASCII value is compared as text, not as raw bytes; otherwise every
		// UTF-16 /Info entry would be a false mismatch against the UTF-8 XMP
		// value. "" correctly means "skip" here, so the bare decoder is right at
		// this site and textStringOrRaw is not.
		infoVal := normalizeXMPText(decodeTextString(dereference(doc, info[m.infoKey])))
		if infoVal == "" {
			continue
		}
		xmpVal := ""
		for _, prop := range m.xmpProps {
			if v := extractXMPValue(xmp, prop); v != "" {
				xmpVal = normalizeXMPText(v)
				break
			}
		}
		if xmpVal == "" {
			continue // XMP counterpart absent/unextractable/ambiguous: not a mismatch
		}
		if infoVal != xmpVal {
			// QuoteToASCII keeps the message ASCII-only (13-1 plain-text
			// contract) even when the decoded values carry non-ASCII text.
			hits = append(hits, ruleHit{
				message: fmt.Sprintf("/Info /%s (%s) differs from XMP %s (%s)",
					m.infoKey, strconv.QuoteToASCII(infoVal), m.xmpProps[0], strconv.QuoteToASCII(xmpVal)),
			})
		}
	}
	return hits
}

// checkDocumentID flags a trailer with no /ID file identifier (PDF/A-1b
// requires a file identifier). Document-level (no object ref).
func checkDocumentID(doc *DocumentState) []ruleHit {
	xrt := doc.PDFContext.XRefTable
	if xrt != nil && len(xrt.ID) > 0 {
		return nil
	}
	return []ruleHit{{message: "document /ID is missing from the trailer"}}
}

// --- PDF/UA-1 structural rule checks -----------------------------------------

// checkMarked flags a document whose catalog lacks /MarkInfo << /Marked true >>
// (PDF/UA-1 requires tagged content). Document-level.
func checkMarked(doc *DocumentState) []ruleHit {
	cat := catalogDict(doc)
	if cat == nil {
		return nil
	}
	mi := asDict(dereference(doc, cat["MarkInfo"]))
	if mi != nil {
		if b, ok := dereference(doc, mi["Marked"]).(pdfcpu_types.Boolean); ok && b.Value() {
			return nil
		}
	}
	return []ruleHit{{message: "document is not marked as tagged (/MarkInfo /Marked true absent)"}}
}

// checkStructTreeRoot flags a document whose catalog lacks a /StructTreeRoot
// (PDF/UA-1 requires a structure tree). Document-level.
func checkStructTreeRoot(doc *DocumentState) []ruleHit {
	cat := catalogDict(doc)
	if cat == nil {
		return nil
	}
	if _, ok := cat["StructTreeRoot"]; ok {
		return nil
	}
	return []ruleHit{{message: "document has no structure tree root (/StructTreeRoot absent)"}}
}

// checkLang flags a document whose catalog lacks a natural-language /Lang entry
// (PDF/UA-1 7.2). Document-level - the canonical "Document" group member.
func checkLang(doc *DocumentState) []ruleHit {
	cat := catalogDict(doc)
	if cat == nil {
		return nil
	}
	if v := stringValue(dereference(doc, cat["Lang"])); v != "" {
		return nil
	}
	return []ruleHit{{message: "document /Lang is missing (no default natural language)"}}
}

// --- shared helpers ----------------------------------------------------------

// forEachObject visits every non-free, non-head xref slot with its object
// number, generation, and resolved object. entry.Object is used when populated;
// otherwise the object is dereferenced.
func forEachObject(doc *DocumentState, fn func(objNum, gen int, obj pdfcpu_types.Object)) {
	xrt := doc.PDFContext.XRefTable
	if xrt == nil {
		return
	}
	nums := make([]int, 0, len(xrt.Table))
	for objNum := range xrt.Table {
		nums = append(nums, objNum)
	}
	slices.Sort(nums) // deterministic problem order
	for _, objNum := range nums {
		entry := xrt.Table[objNum]
		if entry == nil || entry.Free || objNum == 0 {
			continue
		}
		gen := 0
		if entry.Generation != nil {
			gen = *entry.Generation
		}
		obj := entry.Object
		if obj == nil {
			resolved, err := doc.PDFContext.Dereference(*pdfcpu_types.NewIndirectRef(objNum, gen))
			if err != nil {
				continue
			}
			obj = resolved
		}
		fn(objNum, gen, obj)
	}
}

// catalogDict returns the document catalog dict, or nil.
func catalogDict(doc *DocumentState) pdfcpu_types.Dict {
	cat, err := doc.PDFContext.Catalog()
	if err != nil {
		return nil
	}
	return cat
}

// dictName returns the /Name value of d[key] (dereferenced), or "".
func dictName(doc *DocumentState, d pdfcpu_types.Dict, key string) string {
	if n, ok := dereference(doc, d[key]).(pdfcpu_types.Name); ok {
		return n.Value()
	}
	return ""
}

// catalogHasNonEmptyArray reports whether the catalog carries a non-empty array
// under key.
func catalogHasNonEmptyArray(doc *DocumentState, key string) bool {
	cat := catalogDict(doc)
	if cat == nil {
		return false
	}
	return len(dereferenceArray(doc, cat[key])) > 0
}

// infoDict returns the trailer /Info dict, or nil.
func infoDict(doc *DocumentState) pdfcpu_types.Dict {
	xrt := doc.PDFContext.XRefTable
	if xrt == nil || xrt.Info == nil {
		return nil
	}
	return asDict(dereference(doc, *xrt.Info))
}

// catalogXMP returns the decoded catalog /Metadata XMP packet, or "".
func catalogXMP(doc *DocumentState) string {
	cat := catalogDict(doc)
	if cat == nil {
		return ""
	}
	sd, ok := dereference(doc, cat["Metadata"]).(pdfcpu_types.StreamDict)
	if !ok {
		return ""
	}
	xmp, _ := decodeXMPStream(sd)
	return xmp
}

// maxDeviceColorFormDepth caps the recursion into nested form XObjects while
// scanning for device color, so a self-referential /Resources chain cannot loop.
const maxDeviceColorFormDepth = 8

// documentUsesDeviceColor reports whether the document sets a device color
// (rg/RG/k/K/g/G operators or an explicit /DeviceRGB / /DeviceCMYK /
// /DeviceGray colorspace name) in any page content stream OR in a form XObject
// reachable from a page's /Resources (recursed, depth-capped). Bounded scan:
// device color used only inside image-XObject colorspaces, annotation
// appearance streams, tiling patterns, or Type3 glyph procs is NOT detected -
// that under-reports (never falsely flags), consistent with the structural
// firewall; veraPDF is the authoritative oracle.
func documentUsesDeviceColor(doc *DocumentState) bool {
	n := doc.PageCount
	if n <= 0 {
		return false
	}
	xrt := doc.PDFContext.XRefTable
	for p := 1; p <= n; p++ {
		var pageDict pdfcpu_types.Dict
		_ = safeCall(func() error {
			pd, _, _, e := xrt.PageDict(p, false)
			pageDict = pd
			return e
		})
		if pageDict == nil {
			continue
		}
		for _, sd := range pageContentStreams(doc, pageDict) {
			if streamUsesDeviceColor(sd) {
				return true
			}
		}
		if formsUseDeviceColor(doc, asDict(dereference(doc, pageDict["Resources"])), 0) {
			return true
		}
	}
	return false
}

// formsUseDeviceColor reports whether any form XObject in a /Resources /XObject
// map (recursed into nested form resources, depth-capped) sets a device color.
func formsUseDeviceColor(doc *DocumentState, resources pdfcpu_types.Dict, depth int) bool {
	if resources == nil || depth > maxDeviceColorFormDepth {
		return false
	}
	xobjs := asDict(dereference(doc, resources["XObject"]))
	if xobjs == nil {
		return false
	}
	for _, v := range xobjs {
		sd, ok := dereference(doc, v).(pdfcpu_types.StreamDict)
		if !ok || dictName(doc, sd.Dict, "Subtype") != "Form" {
			continue
		}
		if streamUsesDeviceColor(sd) {
			return true
		}
		if formsUseDeviceColor(doc, asDict(dereference(doc, sd.Dict["Resources"])), depth+1) {
			return true
		}
	}
	return false
}

// pageContentStreams resolves a page's /Contents (single ref or array of refs)
// to its StreamDicts.
func pageContentStreams(doc *DocumentState, page pdfcpu_types.Dict) []pdfcpu_types.StreamDict {
	var out []pdfcpu_types.StreamDict
	c := page["Contents"]
	if c == nil {
		return out
	}
	add := func(o pdfcpu_types.Object) {
		if sd, ok := dereference(doc, o).(pdfcpu_types.StreamDict); ok {
			out = append(out, sd)
		}
	}
	if arr := dereferenceArray(doc, c); arr != nil {
		for _, e := range arr {
			add(e)
		}
		return out
	}
	add(c)
	return out
}

// streamUsesDeviceColor tokenizes a decoded content stream and reports whether
// it sets a device color.
func streamUsesDeviceColor(sd pdfcpu_types.StreamDict) bool {
	if e := safeCall(func() error { return sd.Decode() }); e != nil {
		return false
	}
	for _, t := range tokenizeContentStream(string(sd.Content)) {
		switch t.Type {
		case "operator":
			if deviceColorOps[t.Value] {
				return true
			}
		case "name":
			switch t.Value {
			case "/DeviceRGB", "/DeviceCMYK", "/DeviceGray":
				return true
			}
		}
	}
	return false
}

// xmpEntityReplacer unescapes the five predefined XML entities so an XMP value
// compares equal to the raw /Info bytes (e.g. XMP "AT&amp;T" -> "AT&T").
var xmpEntityReplacer = strings.NewReplacer(
	"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'",
)

// numericEntityRE matches XML numeric character references (decimal &#233; or
// hex &#xE9;), which XML serializers routinely use for non-ASCII text.
var numericEntityRE = regexp.MustCompile(`&#(x[0-9A-Fa-f]+|[0-9]+);`)

// unescapeXMLEntities decodes numeric character references AND the five
// predefined XML entities so an XMP value compares equal to the decoded /Info
// text regardless of how the serializer escaped it (e.g. "Caf&#233;" -> "Café").
// Numeric refs are decoded first so a literal "&amp;#233;" is not mis-decoded.
func unescapeXMLEntities(s string) string {
	s = numericEntityRE.ReplaceAllStringFunc(s, func(m string) string {
		body := m[2 : len(m)-1] // strip leading "&#" and trailing ";"
		base, digits := 10, body
		if body[0] == 'x' || body[0] == 'X' {
			base, digits = 16, body[1:]
		}
		n, err := strconv.ParseInt(digits, base, 32)
		if err != nil || n < 0 || n > 0x10FFFF {
			return m // leave malformed/out-of-range refs untouched
		}
		return string(rune(n))
	})
	return xmpEntityReplacer.Replace(s)
}

// rdfLiRE matches an <rdf:li ...>value</rdf:li> element inside an XMP container.
var rdfLiRE = regexp.MustCompile(`<rdf:li[^>]*>([^<]*)</rdf:li>`)

// extractXMPValue pulls the single text value for an XMP property from a packet
// and unescapes XML entities. Handles the simple element form
// (<pdf:Producer>V</pdf:Producer>) and the rdf:Alt/rdf:Seq container form used
// by dc: properties. A container with MORE THAN ONE rdf:li (e.g. a multi-author
// dc:creator Seq) is ambiguous against a single /Info string, so it returns ""
// (skip) rather than risk a false mismatch. Best-effort: returns "" when the
// property is absent or its value cannot be unambiguously extracted.
func extractXMPValue(xmp, prop string) string {
	// Simple element form.
	if m := regexp.MustCompile(`<`+regexp.QuoteMeta(prop)+`[^>]*>([^<]*)</`+regexp.QuoteMeta(prop)+`>`).FindStringSubmatch(xmp); m != nil {
		if v := strings.TrimSpace(m[1]); v != "" {
			return unescapeXMLEntities(v)
		}
	}
	// Container form: <prop> ... <rdf:li ...>V</rdf:li> ... </prop>.
	block := regexp.MustCompile(`(?s)<` + regexp.QuoteMeta(prop) + `[^>]*>(.*?)</` + regexp.QuoteMeta(prop) + `>`).FindStringSubmatch(xmp)
	if block == nil {
		return ""
	}
	lis := rdfLiRE.FindAllStringSubmatch(block[1], -1)
	if len(lis) != 1 {
		// Zero (nothing extractable) or multiple (ambiguous multi-value): skip.
		return ""
	}
	return unescapeXMLEntities(strings.TrimSpace(lis[0][1]))
}

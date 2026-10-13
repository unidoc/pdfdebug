package naming_and_comment_hygiene_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// rulesDocument is the in-repo record of the naming and comment rules. Without
// it, an agent session that does not load the maintainer's global config
// regenerates the numbered pattern the moment the next suite is written, and the
// sweep is a bailing bucket.
const rulesDocument = "CLAUDE.md"

// TestRulesDocumentNamesTheBannedShapes checks the rules landed in the repo, by
// the one property that does not depend on wording: a rule that bans a shape has
// to spell that shape out, so the document must contain an instance of each shape
// it bans. This is also why the residue scan skips this one file - it is the only
// place in the tree where these strings are the content rather than residue.
//
// The underscore form is not required here: the rule text names the dot-dash
// spelling, and the underscore form is the same rule applied to a function name.
func TestRulesDocumentNamesTheBannedShapes(t *testing.T) {
	root := projectRoot(t)
	data, err := os.ReadFile(filepath.Join(root, rulesDocument))
	if err != nil {
		t.Fatalf("read %s: %v", rulesDocument, err)
	}
	text := string(data)

	shapes := []struct {
		what string
		re   *regexp.Regexp
	}{
		{"a scenario ID in dot-dash form", scenarioIDDotDash},
		{"an acceptance-criterion citation", acceptanceCriterionTag},
		{"a priority tag", priorityTag},
		{"a risk ID", riskID},
		{"a numbered acceptance-suite directory path", numberedSuitePath},
	}
	var missing []string
	for _, s := range shapes {
		if !s.re.MatchString(text) {
			missing = append(missing, s.what+" (pattern "+s.re.String()+")")
		}
	}
	if len(missing) > 0 {
		t.Errorf("%s must state the naming and comment rules, naming each banned shape by example so a reader can\n"+
			"recognise it. Shapes with no example in the document: %s", rulesDocument, reportPaths(missing))
	}
}

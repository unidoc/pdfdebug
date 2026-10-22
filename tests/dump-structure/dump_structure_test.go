package dump_structure_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestDumpStructureJSON_CleanFile(t *testing.T) {
	r, raw := dumpStructureJSON(t, "structure.pdf")

	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &top); err != nil {
		t.Fatal(err)
	}
	for _, k := range reportKeys {
		if _, ok := top[k]; !ok {
			t.Errorf("--json has no %q key", k)
		}
	}

	if !r.HasStructTree || !r.HasParentTree {
		t.Errorf("hasStructTree %v hasParentTree %v, want both true", r.HasStructTree, r.HasParentTree)
	}
	if len(r.Elements) != 1 || r.Elements[0].Element == nil || r.Elements[0].Element.S != "Document" {
		t.Fatalf("elements = %v, want one Document element", kidS(r.Elements))
	}
	doc := r.Elements[0].Element
	if got, want := kidS(doc.Kids), []string{"Heading1", "Figure", "Sect", "Link"}; !slices.Equal(got, want) {
		t.Errorf("Document kids = %v, want %v", got, want)
	}
	pages := map[string]int{}
	eachKid(r.Elements, func(k *structKid) {
		if k.Element != nil {
			pages[k.Element.S] = k.Element.Page
		}
		if k.Kind == "error" {
			t.Errorf("error kid on a clean file: %+v", *k)
		}
	})
	for s, want := range map[string]int{"Heading1": 1, "Figure": 1, "Sect": 2, "P": 2, "Link": 2} {
		if pages[s] != want {
			t.Errorf("%s page = %d, want %d", s, pages[s], want)
		}
	}

	if len(r.Pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(r.Pages))
	}
	for _, n := range []int{1, 2} {
		p := page(t, r, n)
		var mcids []int
		for _, a := range p.Alignment {
			mcids = append(mcids, a.MCID)
			if !a.Consistent {
				t.Errorf("page %d MCID %d not consistent: %+v", n, a.MCID, a)
			}
		}
		if !slices.Equal(mcids, []int{0, 1}) {
			t.Errorf("page %d alignment MCIDs = %v, want [0 1]", n, mcids)
		}
	}
	if len(r.OtherParentTree) != 1 || r.OtherParentTree[0].Key != 2 || r.OtherParentTree[0].S != "Link" {
		t.Errorf("otherParentTree = %+v, want key 2 -> Link", r.OtherParentTree)
	}
	if len(r.Problems) != 0 {
		t.Errorf("problems = %+v, want none", r.Problems)
	}
}

func TestDumpStructureJSON_BrokenFileShowsFindings(t *testing.T) {
	r, _ := dumpStructureJSON(t, "structure-broken.pdf")

	var cycle bool
	eachKid(r.Elements, func(k *structKid) {
		if k.Kind == "error" && strings.Contains(k.Errmsg, "already listed") {
			cycle = true
		}
	})
	if !cycle {
		t.Error("no \"already listed\" error kid for the /K cycle")
	}
	if p2 := page(t, r, 2); p2.ParentTreeErr == "" {
		t.Error("page 2 has no parentTreeError for /StructParents 5")
	}
	var inconsistent bool
	for _, a := range page(t, r, 1).Alignment {
		if !a.Consistent {
			inconsistent = true
		}
	}
	if !inconsistent {
		t.Error("page 1 has no inconsistent alignment row")
	}
	if len(r.Problems) == 0 {
		t.Error("problems is empty, want the /Limits finding")
	}
}

func TestDumpStructureJSON_UntaggedFile(t *testing.T) {
	r, raw := dumpStructureJSON(t, "untagged.pdf")
	if r.HasStructTree {
		t.Error("hasStructTree = true for a file with no /StructTreeRoot")
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &top); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"elements", "roleMap", "otherParentTree", "problems", "pages"} {
		if !strings.HasPrefix(string(top[k]), "[") {
			t.Errorf("%q = %s, want an array", k, top[k])
		}
	}
	if len(r.Pages) == 0 {
		t.Error("no pages listed")
	}
}

func TestDumpStructureJSON_PrettyIsIndented(t *testing.T) {
	stdout, stderr, code := runCLI(t, "dump", "structure", "--json", "--pretty", fixture(t, "structure.pdf"))
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "\n  \"") {
		t.Errorf("--pretty output is not indented:\n%s", stdout)
	}
	var r structureReport
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Errorf("--pretty output is not JSON: %v", err)
	}
}

func TestDumpStructurePlain_CleanFile(t *testing.T) {
	out := dumpStructurePlain(t, "structure.pdf")
	for _, prefix := range []string{"Structure tree:", "Document", "Page 1"} {
		if !hasLinePrefix(out, prefix) {
			t.Errorf("no line starting %q in plain output:\n%s", prefix, out)
		}
	}
	for i, b := range []byte(out) {
		if b > 0x7e || (b < 0x20 && b != '\n' && b != '\t') {
			t.Errorf("plain output byte %d is 0x%02x, want printable ASCII", i, b)
			break
		}
	}
}

func TestDumpStructurePlain_BrokenFileMarksInconsistentRows(t *testing.T) {
	out := dumpStructurePlain(t, "structure-broken.pdf")
	var starred bool
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasSuffix(strings.TrimRight(line, " "), " *") {
			starred = true
		}
	}
	if !starred {
		t.Errorf("no alignment row marked with *:\n%s", out)
	}
	if !hasLinePrefix(out, "* = ") {
		t.Errorf("no \"* = ...\" legend line:\n%s", out)
	}
}

func TestDumpStructurePlain_UntaggedFile(t *testing.T) {
	out := dumpStructurePlain(t, "untagged.pdf")
	if !strings.Contains(out, "No structure tree (/StructTreeRoot absent)") {
		t.Errorf("plain output does not say the structure tree is absent:\n%s", out)
	}
}

func TestDumpStructure_HelpListsCommand(t *testing.T) {
	_, stderr, _ := runCLI(t, "--help")
	if !hasLinePrefix(stderr, "dump structure [--json] [--pretty] <file>") {
		t.Errorf("--help has no `dump structure [--json] [--pretty] <file>` line:\n%s", stderr)
	}
	if !strings.Contains(stderr, "pdfdebug dump structure file.pdf") {
		t.Errorf("--help Examples has no `pdfdebug dump structure file.pdf` line:\n%s", stderr)
	}
}

func TestDumpStructure_UsageErrors(t *testing.T) {
	path := fixture(t, "untagged.pdf")
	cases := map[string][]string{
		"no file":      {"dump", "structure"},
		"two files":    {"dump", "structure", path, path},
		"unknown flag": {"dump", "structure", "--page", "1", path},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, code := runCLI(t, args...)
			if code != 1 {
				t.Errorf("exit %d, want 1", code)
			}
			if !strings.Contains(stderr, "Usage: pdfdebug dump structure") {
				t.Errorf("stderr must carry the dump structure usage line, got: %s", stderr)
			}
			if stdout != "" {
				t.Errorf("a usage error writes nothing to stdout, got: %s", stdout)
			}
		})
	}
}

func TestDumpStructure_MissingFileExits2WithJSONError(t *testing.T) {
	missing := t.TempDir() + "/does-not-exist.pdf"
	stdout, stderr, code := runCLI(t, "dump", "structure", missing)
	if code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	var obj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &obj); err != nil || obj["error"] == "" {
		t.Errorf("stderr = %q, want a JSON {\"error\": ...} object", stderr)
	}
	_, objErr, objCode := runCLI(t, "dump", "objects", missing)
	if objCode != code || strings.TrimSpace(objErr) != strings.TrimSpace(stderr) {
		t.Errorf("dump structure on a missing file: exit %d stderr %q; dump objects: exit %d stderr %q", code, stderr, objCode, objErr)
	}
}

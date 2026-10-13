package object_tree_scalar_values_test

import (
	"strings"
	"testing"
)

// dump source reserializes PDF syntax and is the escape hatch for raw bytes
// once the display surfaces decode. Its bytes are frozen: the expectations
// below are the reserialized form, unaffected by any display decoding.

const structElemSource = `10 0 obj
<<
    /A <<
        /ColSpan 3
        /O /Table
        /RowSpan 2
    >>
    /Alt <FEFF0052006100700070006F00720074002000630065006C006C>
    /E (\376\377\000E\000x\000p)
    /S /TD
    /Type /StructElem
>>
endobj
`

const signatureSource = `12 0 obj
<<
    /ByteRange [ 0 100 200 300 ]
    /Cert <CDCDCDCDCDCDCDCDCDCDCDCDCDCDCDCD>
    /Contents <ABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABAB>
    /SubFilter /adbe.pkcs7.detached
    /Type /Sig
>>
endobj
`

func TestSource_TextStringsAreStillReserializedByteExact(t *testing.T) {
	stdout, stderr, code := runCLI(t, "dump", "source", "--ref", "10 0 R", fixturePath(t, "scalar-values.pdf"))
	if code != 0 {
		t.Fatalf("dump source exited %d: %s", code, stderr)
	}
	if stdout != structElemSource {
		t.Errorf("dump source output moved.\n--- got ---\n%s\n--- want ---\n%s", stdout, structElemSource)
	}
}

func TestSource_BinaryStringsAreStillReserializedInFull(t *testing.T) {
	stdout, stderr, code := runCLI(t, "dump", "source", "--ref", "12 0 R", fixturePath(t, "scalar-values.pdf"))
	if code != 0 {
		t.Fatalf("dump source exited %d: %s", code, stderr)
	}
	if stdout != signatureSource {
		t.Errorf("dump source output moved.\n--- got ---\n%s\n--- want ---\n%s", stdout, signatureSource)
	}
	if strings.Contains(stdout, "<binary,") {
		t.Errorf("dump source picked up the display summary; it serializes PDF syntax")
	}
}

func TestSource_ControlCharactersAreStillReserializedUnescaped(t *testing.T) {
	stdout, stderr, code := runCLI(t, "dump", "source", "--ref", "6 0 R", fixturePath(t, "scalar-values.pdf"))
	if code != 0 {
		t.Fatalf("dump source exited %d: %s", code, stderr)
	}

	// The stored escape sequences, byte for byte, not the display escaping.
	for _, want := range []string{`/Newline (a\nb)`, `/Curly (don\222t)`, `/Nul (a\000b)`} {
		if !strings.Contains(stdout, want) {
			t.Errorf("dump source no longer emits %q\n--- output ---\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, `\x92`) {
		t.Errorf("dump source picked up the display escaping")
	}
}

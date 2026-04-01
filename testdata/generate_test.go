package testdata_test

import (
	"fmt"
	"os"
	"testing"

	pdfcpu_api "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// minimalPDFContent returns a valid single-page PDF as raw bytes.
func minimalPDFContent() []byte {
	pdf := "%PDF-1.4\n"

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n"

	body := pdf + obj1 + obj2 + obj3

	o1 := len(pdf)
	o2 := o1 + len(obj1)
	o3 := o2 + len(obj2)
	xrefOffset := len(body)

	xref := fmt.Sprintf("xref\n0 4\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n",
		o1, o2, o3)
	trailer := fmt.Sprintf("trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	return []byte(body + xref + trailer)
}

// multipagePDFContent returns a valid 3-page PDF as raw bytes.
func multipagePDFContent() []byte {
	pdf := "%PDF-1.4\n"

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n"
	obj4 := "4 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n"
	obj5 := "5 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n"

	body := pdf + obj1 + obj2 + obj3 + obj4 + obj5

	o1 := len(pdf)
	o2 := o1 + len(obj1)
	o3 := o2 + len(obj2)
	o4 := o3 + len(obj3)
	o5 := o4 + len(obj4)
	xrefOffset := len(body)

	xref := fmt.Sprintf("xref\n0 6\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n",
		o1, o2, o3, o4, o5)
	trailer := fmt.Sprintf("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	return []byte(body + xref + trailer)
}

// TestGenerateFixtures creates test PDF files used by the test suite.
// Run with: go test -run TestGenerateFixtures -v ./testdata/
func TestGenerateFixtures(t *testing.T) {
	t.Run("minimal.pdf", func(t *testing.T) {
		if _, err := os.Stat("minimal.pdf"); err == nil {
			t.Skip("minimal.pdf already exists")
		}
		if err := os.WriteFile("minimal.pdf", minimalPDFContent(), 0644); err != nil {
			t.Fatalf("failed to create minimal.pdf: %v", err)
		}
		// Verify pdfcpu can read it
		ctx, err := pdfcpu_api.ReadContextFile("minimal.pdf")
		if err != nil {
			os.Remove("minimal.pdf")
			t.Fatalf("minimal.pdf is not valid according to pdfcpu: %v", err)
		}
		t.Logf("minimal.pdf created: %d pages", ctx.PageCount)
	})

	t.Run("multipage.pdf", func(t *testing.T) {
		if _, err := os.Stat("multipage.pdf"); err == nil {
			t.Skip("multipage.pdf already exists")
		}
		if err := os.WriteFile("multipage.pdf", multipagePDFContent(), 0644); err != nil {
			t.Fatalf("failed to create multipage.pdf: %v", err)
		}
		ctx, err := pdfcpu_api.ReadContextFile("multipage.pdf")
		if err != nil {
			os.Remove("multipage.pdf")
			t.Fatalf("multipage.pdf is not valid according to pdfcpu: %v", err)
		}
		t.Logf("multipage.pdf created: %d pages", ctx.PageCount)
	})

	t.Run("malformed.pdf", func(t *testing.T) {
		if _, err := os.Stat("malformed.pdf"); err == nil {
			t.Skip("malformed.pdf already exists")
		}
		data := []byte("%PDF-1.4\n%%EOF\ngarbage xref corrupted data truncated")
		if err := os.WriteFile("malformed.pdf", data, 0644); err != nil {
			t.Fatalf("failed to create malformed.pdf: %v", err)
		}
	})

	t.Run("encrypted.pdf", func(t *testing.T) {
		if _, err := os.Stat("encrypted.pdf"); err == nil {
			t.Skip("encrypted.pdf already exists")
		}
		// Ensure minimal.pdf exists first
		if _, err := os.Stat("minimal.pdf"); os.IsNotExist(err) {
			if err := os.WriteFile("minimal.pdf", minimalPDFContent(), 0644); err != nil {
				t.Fatalf("failed to create minimal.pdf for encryption: %v", err)
			}
		}
		conf := pdfcpu_model.NewAESConfiguration("testpass", "ownerpass", 256)
		if err := pdfcpu_api.EncryptFile("minimal.pdf", "encrypted.pdf", conf); err != nil {
			t.Fatalf("failed to create encrypted.pdf: %v", err)
		}
	})
}

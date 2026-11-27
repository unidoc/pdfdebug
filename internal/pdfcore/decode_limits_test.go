package pdfcore

import (
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/filter"
	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Every image ceiling pdfdebug allows fits inside pdfcpu's default resource
// limits, so pdfcpu never refuses an image pdfdebug lets through. A pdfcpu that
// tightens a default fails here.
func TestImageCeilingsFitPdfcpuDefaultLimits(t *testing.T) {
	limits := pdfcpu_model.DefaultResourceLimits()

	if int64(maxImageDecodeBytes) > filter.DefaultMaxDecodeBytes {
		t.Errorf("maxImageDecodeBytes %d exceeds filter.DefaultMaxDecodeBytes %d",
			int64(maxImageDecodeBytes), filter.DefaultMaxDecodeBytes)
	}
	if int64(maxImagePixels) > limits.MaxImagePixels {
		t.Errorf("maxImagePixels %d exceeds DefaultResourceLimits().MaxImagePixels %d",
			int64(maxImagePixels), limits.MaxImagePixels)
	}
	if int64(maxImageDecodeBytes) > limits.MaxImageBytes {
		t.Errorf("maxImageDecodeBytes %d exceeds DefaultResourceLimits().MaxImageBytes %d",
			int64(maxImageDecodeBytes), limits.MaxImageBytes)
	}
}

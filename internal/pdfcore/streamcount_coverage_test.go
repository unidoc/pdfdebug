// Tests for measurement coverage in the per-stage stream counter. countStages
// returns a pipelineMeasurement{measured, firstUnmeasured} alongside the ceiling
// verdict: measured is true only when every runnable filter was counted, and
// firstUnmeasured names the first that was not. These cases pin that coverage
// axis, the CCITTFaxDecode geometry sizing, and the filter-name classification.
//
// The coverage axis is SEPARATE from the chainContinues axis filterReader
// returns: a stage that ends the chain is still measured; only a runnable filter
// with no counter is unmeasured, and so is anything after a chain-ending stage.
package pdfcore

import (
	"errors"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/filter"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ccittStream builds a StreamDict whose sole filter is CCITTFaxDecode. height is
// written as the /Height dict entry (0 leaves it absent); parms is the filter's
// DecodeParms. The raw body is arbitrary because the CCITT stage is sized from
// its parameters, not decoded.
func ccittStream(height int, parms pdfcpu_types.Dict) *pdfcpu_types.StreamDict {
	dict := pdfcpu_types.Dict{}
	if height > 0 {
		dict["Height"] = pdfcpu_types.Integer(height)
	}
	return &pdfcpu_types.StreamDict{
		Dict:           dict,
		FilterPipeline: []pdfcpu_types.PDFFilter{{Name: "CCITTFaxDecode", DecodeParms: parms}},
		Raw:            []byte("ccitt encoded bytes"),
	}
}

// A pipeline whose first runnable filter is one the counter does not model
// reports the pipeline UNMEASURED rather than measured-and-in-bounds, so a
// caller can tell "could not measure" apart from "measured, fits". A counter
// that reported such a pipeline as measured would defeat this test.
func TestCountStages_UnmodelledFirstFilterReportsUnmeasured(t *testing.T) {
	// 4-component DCTDecode is runnable (not a stopping filter) yet unmodelled,
	// so it reaches the zero-counter branch. JBIG2 would be decided by pdfcpu's
	// ErrUnsupportedFilter instead, which is why DCT is the fixture.
	sd := &pdfcpu_types.StreamDict{
		Dict:           pdfcpu_types.Dict{},
		FilterPipeline: []pdfcpu_types.PDFFilter{{Name: "DCTDecode"}},
		Raw:            []byte("not really a jpeg"),
		CSComponents:   4,
	}

	m, err := countStages(sd, 1024)
	if err != nil {
		t.Fatalf("an unmodelled sole filter must not be refused by the count, got %v", err)
	}
	if m.measured {
		t.Fatalf("a pipeline led by an unmodelled filter must report UNMEASURED, got measured")
	}
	if m.firstUnmeasured != "DCTDecode" {
		t.Errorf("expected the unmeasured filter to be named DCTDecode, got %q", m.firstUnmeasured)
	}
}

// A pipeline the counter can size at every runnable stage reports MEASURED. Both
// filters are modelled and the chain runs through, so coverage is end to end.
func TestCountStages_FullyModelledPipelineReportsMeasured(t *testing.T) {
	payload := []byte("ascii85 over flate, both modelled")
	sd := pipeline(ascii85Wrap(zlibBytes(t, payload)), "ASCII85Decode", "FlateDecode")

	// The limit clears the zlib intermediate (larger than the tiny payload), so
	// coverage is what the assertion turns on, not the ceiling.
	m, err := countStages(sd, 64*1024)
	if err != nil {
		t.Fatalf("an in-bounds fully modelled pipeline must not be refused, got %v", err)
	}
	if !m.measured {
		t.Errorf("a fully modelled in-bounds pipeline must report MEASURED, got unmeasured (%q)", m.firstUnmeasured)
	}
}

// A stage that ends the chain but is the last runnable filter leaves the
// pipeline MEASURED: its own output is counted and nothing follows it. This is
// the coverage axis, distinct from chainContinues - the CCITT stage sets
// chainContinues false yet the pipeline is fully covered.
func TestCountStages_SoleChainEndingCCITTIsMeasured(t *testing.T) {
	// ceil(1728/8) * 100 = 216 * 100 = 21600 bytes, comfortably in bounds.
	sd := ccittStream(0, pdfcpu_types.Dict{
		"Columns": pdfcpu_types.Integer(1728),
		"Rows":    pdfcpu_types.Integer(100),
	})

	m, err := countStages(sd, 50*1024)
	if err != nil {
		t.Fatalf("an in-bounds CCITT stream must not be refused, got %v", err)
	}
	if !m.measured {
		t.Errorf("a sole CCITT stage is the whole runnable pipeline and must report MEASURED, got unmeasured (%q)", m.firstUnmeasured)
	}
}

// A filter AFTER a chain-ending CCITT stage is unmeasured, so the pipeline
// reports UNMEASURED and names that filter. The CCITT stage hands only a byte
// COUNT downstream, not the decoded bytes, so a stage reading its output would
// mis-measure; ending the chain routes the successor to the explicit unmeasured
// signal instead of a silent zero count.
func TestCountStages_FilterAfterCCITTIsReportedUnmeasured(t *testing.T) {
	sd := &pdfcpu_types.StreamDict{
		Dict: pdfcpu_types.Dict{},
		FilterPipeline: []pdfcpu_types.PDFFilter{
			{Name: "CCITTFaxDecode", DecodeParms: pdfcpu_types.Dict{
				"Columns": pdfcpu_types.Integer(1728),
				"Rows":    pdfcpu_types.Integer(100),
			}},
			{Name: "FlateDecode"},
		},
		Raw: []byte("ccitt then flate"),
	}

	m, err := countStages(sd, 50*1024)
	if err != nil {
		t.Fatalf("the CCITT stage is in bounds, so the count must not refuse the pipeline, got %v", err)
	}
	if m.measured {
		t.Fatalf("a filter after the chain-ending CCITT stage is unmeasured, so the pipeline must report UNMEASURED")
	}
	if m.firstUnmeasured != "FlateDecode" {
		t.Errorf("expected the filter after CCITT to be named as unmeasured, got %q", m.firstUnmeasured)
	}
}

// CCITT output is one bit per pixel packed by row: ceil(Columns/8) * Rows bytes.
// The count is emitted without a decode, so an over-ceiling geometry is rejected
// before allocation and an in-bounds one is measured.
func TestCountStages_CCITTOutputIsCountedFromColumnsAndRows(t *testing.T) {
	for _, c := range []struct {
		name        string
		columns     int
		rows        int
		limit       int64
		wantRefused bool
	}{
		// ceil(8000/8)=1000 bytes/row * 100000 rows = 100 MB, over a 4 MiB ceiling.
		{"over ceiling", 8000, 100000, 4 * 1024 * 1024, true},
		// 216 * 100 = 21600 bytes, exactly at a 21600 ceiling.
		{"exactly at ceiling", 1728, 100, 21600, false},
		// One byte under the same output is refused.
		{"one byte under ceiling", 1728, 100, 21599, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			sd := ccittStream(0, pdfcpu_types.Dict{
				"Columns": pdfcpu_types.Integer(c.columns),
				"Rows":    pdfcpu_types.Integer(c.rows),
			})
			_, err := countStages(sd, c.limit)
			if c.wantRefused {
				if !errors.Is(err, ErrUnsupportedPDF) {
					t.Fatalf("expected the CCITT geometry to be refused, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("an in-bounds CCITT geometry must not be refused, got %v", err)
			}
		})
	}
}

// When /Rows is absent the CCITT stage falls back to the image /Height, matching
// pdfcpu's fixParms. /Height is a StreamDict dict entry, not a decode-param, so
// the counter has to reach it: a stream sized from Height alone must be refused
// when that height makes it over-ceiling.
func TestCountStages_CCITTRowsFallBackToHeight(t *testing.T) {
	// No /Rows in DecodeParms; /Height supplies the row count.
	// ceil(8000/8)=1000 bytes/row * 100000 = 100 MB, over a 4 MiB ceiling.
	sd := ccittStream(100000, pdfcpu_types.Dict{"Columns": pdfcpu_types.Integer(8000)})

	if _, err := countStages(sd, 4*1024*1024); !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("a CCITT stream sized from its /Height fallback must be refused when over ceiling, got %v", err)
	}
}

// /Columns is absent, so it takes the PDF default of 1728.
// ceil(1728/8)=216 bytes/row * 100000 = 21.6 MB, over a 4 MiB ceiling.
func TestCountStages_CCITTColumnsDefaultTo1728(t *testing.T) {
	sd := ccittStream(0, pdfcpu_types.Dict{"Rows": pdfcpu_types.Integer(100000)})

	if _, err := countStages(sd, 4*1024*1024); !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("a CCITT stream missing /Columns must be sized at the 1728 default and refused, got %v", err)
	}
}

// /BlackIs1 changes the bit polarity, not the byte count, so it must not move the
// verdict. The same geometry stays in bounds with it set.
func TestCountStages_CCITTBlackIs1DoesNotChangeCount(t *testing.T) {
	parms := pdfcpu_types.Dict{
		"Columns":  pdfcpu_types.Integer(1728),
		"Rows":     pdfcpu_types.Integer(100),
		"BlackIs1": pdfcpu_types.Integer(1),
	}
	if _, err := countStages(ccittStream(0, parms), 21600); err != nil {
		t.Errorf("/BlackIs1 must not change the counted byte size, got %v", err)
	}
}

// Every filter name pdfcpu defines today is classified: the counter either
// models its own stage output or lists it as unmodelled. Referencing the names
// by const means a renamed, removed, or revalued pdfcpu constant breaks this
// test. A name filterModelled does not classify is treated as unmodelled at
// runtime (filterReader's default); this test pins that classification for the
// known set, but cannot see a brand-new pdfcpu const that no code references yet.
func TestFilterClassification_EveryPDFCPUFilterIsCountedOrUnmodelled(t *testing.T) {
	// The nine filter-name consts pdfcpu exports from pkg/filter/filter.go,
	// referenced by const, paired with whether the counter models each one's
	// output.
	want := map[string]bool{
		filter.ASCII85:   true,
		filter.ASCIIHex:  true,
		filter.RunLength: true,
		filter.LZW:       true,
		filter.Flate:     true,
		filter.CCITTFax:  true,
		filter.JBIG2:     false,
		filter.DCT:       false,
		filter.JPX:       false,
	}

	for name, modelled := range want {
		got, known := filterModelled[name]
		if !known {
			t.Errorf("filter %q is not classified; every pdfcpu filter must be counted or explicitly unmodelled", name)
			continue
		}
		if got != modelled {
			t.Errorf("filter %q: classified modelled=%v, want %v", name, got, modelled)
		}
	}

	if len(filterModelled) != len(want) {
		t.Errorf("filterModelled has %d entries, want exactly the %d pdfcpu filter names", len(filterModelled), len(want))
	}

	// A name pdfcpu does not define must not be classified, which is what forces
	// the list to be explicit rather than a catch-all default.
	if _, known := filterModelled["NotARealDecode"]; known {
		t.Error("an unknown filter name must not be classified, or the classification is a fail-open default")
	}
}

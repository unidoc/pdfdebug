package pdfcore

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync/atomic"

	pdfcpu_api "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// pdfcpuCheckValidate is the validate step of the pdfcpu check, run on the
// check's own parse. Tests replace it.
var pdfcpuCheckValidate = pdfcpu_api.ValidateContextWithReport

// pdfcpuCheckParses counts the parses the pdfcpu check starts.
var pdfcpuCheckParses atomic.Int64

// pdfcpuCheckSem lets one pdfcpu check parse run at a time in the process, so
// opening many files queues their checks instead of holding many extra parses
// in memory.
var pdfcpuCheckSem = make(chan struct{}, 1)

// pdfcpuRun is what one pdfcpu check run produced: the notices of the
// validation report and the validator's fatal error, or degraded, the reason
// neither rule could be evaluated.
type pdfcpuRun struct {
	notices  []pdfcpu_model.ValidationNotice
	fatal    error
	degraded error
}

// validatePdfcpu returns the pdfcpu profile's result for doc, built once per
// DocumentState. A run that failed (close, ErrFileChanged, ErrFileBusy) is
// returned as an error and not cached; a degraded run is a cached result.
func validatePdfcpu(doc *DocumentState) (*ValidationResult, error) {
	return doc.pdfcpuCheck.get(func() (*ValidationResult, error) {
		run, err := runPdfcpuCheck(doc)
		if err != nil {
			return nil, err
		}
		return pdfcpuResult(doc, run), nil
	})
}

// runPdfcpuCheck parses doc's file again through the read guard, with the
// same parse Open uses, and runs pdfcpu's validator in relaxed mode on that
// parse. A checksum correction the parse needed comes first in the notices. It waits
// for the semaphore before opening the file, never holds doc.pdfMu and does
// not touch the displayed context. Closing the document cancels the wait, the
// parse and the validation, and returns an error satisfying
// errors.Is(err, context.Canceled).
func runPdfcpuCheck(doc *DocumentState) (pdfcpuRun, error) {
	ctx := doc.closeCtx
	select {
	case pdfcpuCheckSem <- struct{}{}:
	case <-ctx.Done():
		return pdfcpuRun{}, ctx.Err()
	}
	defer func() { <-pdfcpuCheckSem }()
	if err := ctx.Err(); err != nil {
		return pdfcpuRun{}, err
	}

	f, err := openVerified(doc.FilePath, doc.fingerprint)
	if err != nil {
		return pdfcpuRun{}, err
	}
	defer func() { _ = f.Close() }()
	pdfcpuCheckParses.Add(1)

	var run pdfcpuRun
	rs := &busySeeker{f: f}
	panicErr := recoverAll(func() {
		pctx, repairs, err := parseFile(ctx, rs, func() ([]byte, error) {
			if _, err := rs.Seek(0, io.SeekStart); err != nil {
				return nil, err
			}
			return io.ReadAll(rs)
		})
		if err != nil {
			if ctx.Err() == nil {
				run.degraded = fmt.Errorf("pdfcpu could not parse the file: %w", err)
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
		pctx.Configuration.ValidationMode = pdfcpu_model.ValidationRelaxed
		report, fatal := pdfcpuCheckValidate(ctx, pctx)
		run.notices, run.fatal = append(repairs, report.Notices()...), fatal
	})
	if panicErr != nil {
		run = pdfcpuRun{degraded: panicErr}
	}
	// pdfcpu returns a cancel as its parse or fatal error, so a close is
	// checked before either is read.
	if err := ctx.Err(); err != nil {
		return pdfcpuRun{}, err
	}
	// A read refused by another process's lock is not a parse failure of the
	// file, so it is returned uncached rather than degrading the result.
	if rs.busy {
		return pdfcpuRun{}, ErrFileBusy
	}
	if err := verifyHandle(f, doc.fingerprint); err != nil {
		return pdfcpuRun{}, err
	}
	return run, nil
}

// busySeeker reads f and records whether a read was refused by another
// process's lock.
type busySeeker struct {
	f    *os.File
	busy bool
}

func (r *busySeeker) Read(p []byte) (int, error) {
	n, err := r.f.Read(p)
	if err != nil && isFileBusy(err) {
		r.busy = true
		err = fmt.Errorf("%w: %w", ErrFileBusy, err)
	}
	return n, err
}

func (r *busySeeker) Seek(offset int64, whence int) (int64, error) {
	return r.f.Seek(offset, whence)
}

// recoverAll runs fn and returns any panic it raised, a runtime.Error
// included, as an error; nil when fn returned. A panic in pdfcpu's validator
// degrades the check rather than crashing the caller.
func recoverAll(fn func()) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("pdfcpu panicked: %v", rec)
		}
	}()
	fn()
	return nil
}

// pdfcpuResult maps a pdfcpu run to the profile's ValidationResult: the fatal
// error is one pdfcpu-validator problem and each notice one pdfcpu-notices
// problem, in report order. The object numbers pdfcpu names get their
// generation from doc's displayed cross-reference table, read under a short
// doc.pdfMu; a number that table does not have gets no ObjRef.
func pdfcpuResult(doc *DocumentState, run pdfcpuRun) *ValidationResult {
	fatalObj := 0
	var ve *pdfcpu_model.ValidationError
	if run.fatal != nil && errors.As(run.fatal, &ve) {
		fatalObj = ve.ObjectNumber()
	}
	objNrs := []int{fatalObj}
	for _, n := range run.notices {
		objNrs = append(objNrs, n.ObjectNumber)
	}
	gens := displayedGenerations(doc, objNrs)
	hit := func(message string, objNr int) ruleHit {
		h := ruleHit{message: message}
		if gen, ok := gens[objNr]; ok {
			h.objRef = fmt.Sprintf("%d %d R", objNr, gen)
			h.objNodeID = fmt.Sprintf("obj:%d:%d", gen, objNr)
		}
		return h
	}

	var fatalHits, noticeHits []ruleHit
	if run.fatal != nil {
		fatalHits = append(fatalHits, hit(run.fatal.Error(), fatalObj))
	}
	for _, n := range run.notices {
		msg := fmt.Sprintf("%s during %s: %s", n.Disposition, n.Phase, n.Message)
		if n.PageNumber > 0 {
			msg += fmt.Sprintf(" (page %d)", n.PageNumber)
		}
		if n.Path != "" {
			msg += fmt.Sprintf(" (path %s)", n.Path)
		}
		noticeHits = append(noticeHits, hit(msg, n.ObjectNumber))
	}

	res := newValidationResult(ProfilePdfcpu)
	for _, r := range ruleRegistry {
		if r.profile != ProfilePdfcpu {
			continue
		}
		hits := noticeHits
		if r.id == "pdfcpu-validator" {
			hits = fatalHits
		}
		addRuleOutcome(res, r, hits, run.degraded)
	}
	res.Summary = summarize(res.Problems)
	return res
}

// displayedGenerations returns the generation of each object number above 0
// in objNrs that doc's displayed cross-reference table holds as an in-use
// entry.
func displayedGenerations(doc *DocumentState, objNrs []int) map[int]int {
	gens := map[int]int{}
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()
	if doc.PDFContext == nil || doc.PDFContext.XRefTable == nil {
		return gens
	}
	for _, objNr := range objNrs {
		if objNr <= 0 {
			continue
		}
		entry := doc.PDFContext.XRefTable.Table[objNr]
		if entry == nil || entry.Free {
			continue
		}
		gen := 0
		if entry.Generation != nil {
			gen = *entry.Generation
		}
		gens[objNr] = gen
	}
	return gens
}

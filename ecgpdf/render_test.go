package ecgpdf

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"
)

// These are acceptance tests for IHE Cardiology TF-2 §4.6.4.2.2 (transaction
// CARD-6), not documentation of it. Each one pins a property the rendered
// document must have for the project's regulatory position to hold; a failure
// here means the PDF stopped satisfying a claim made in writing to the ANSM,
// not that a constant drifted.

// --- scale and grid -------------------------------------------------------

// The print scale is the clinical standard and is declared as such. It is not
// a layout knob: widening it to use spare paper silently rescales every
// measurement a clinician takes off the printout.
func TestPrintScaleIsClinicalStandard(t *testing.T) {
	if mmPerSec != 25.0 {
		t.Errorf("paper speed = %g mm/s, want 25", mmPerSec)
	}
	if mmPerMV != 10.0 {
		t.Errorf("gain = %g mm/mV, want 10", mmPerMV)
	}
}

// CARD-6 requires a fixed aspect ratio so grid squares stay square. That falls
// out of the scale, so assert the derived pitches rather than the constants.
func TestGridSquaresAreSquare(t *testing.T) {
	minorX := 0.04 * mmPerSec // one minor square in time
	minorY := 0.10 * mmPerMV  // one minor square in amplitude
	if minorX != minorY {
		t.Fatalf("minor square is %g × %g mm, want square", minorX, minorY)
	}
	if minorX != 1.0 {
		t.Errorf("minor square = %g mm, want 1.0", minorX)
	}
	if 5*minorX != 5.0 {
		t.Errorf("major square = %g mm, want 5.0", 5*minorX)
	}
}

func TestCalibrationPulseGeometry(t *testing.T) {
	if calWidthMM != 5.0 {
		t.Errorf("pulse width = %g mm, want 5.0 (200 ms at 25 mm/s)", calWidthMM)
	}
	if calHeightMM != 10.0 {
		t.Errorf("pulse height = %g mm, want 10.0 (1 mV at 10 mm/mV)", calHeightMM)
	}
	if calLeadInMM+calWidthMM > calW {
		t.Errorf("pulse (%g + %g mm) does not fit the %g mm lead-in", calLeadInMM, calWidthMM, calW)
	}
	// t=0 must land on a major grid line, else the ruling is offset from the trace.
	if rem := calW / 5.0; rem != float64(int(rem)) {
		t.Errorf("lead-in %g mm is not a multiple of the 5 mm major pitch", calW)
	}
}

func TestTraceAreaIsTenSeconds(t *testing.T) {
	if got := traceW / mmPerSec; got != 10.0 {
		t.Errorf("trace area = %g s, want 10", got)
	}
	if got := (traceW / 4) / mmPerSec; got != 2.5 {
		t.Errorf("column = %g s, want 2.5", got)
	}
	// The whole ruled area plus margins must fit A4 landscape (297 mm).
	if gridX+gridW+margin > 297.0 {
		t.Errorf("ruled area overflows the page: %g mm", gridX+gridW+margin)
	}
}

// --- rendered document ----------------------------------------------------

// A rasterised trace is non-conformant. Two independent checks, because either
// alone is weak: no image XObject may be *defined* in the file, and no XObject
// may be *painted* by the content stream. (Matching "/Image" alone would be a
// false positive — every fpdf file lists /ImageB /ImageC /ImageI in its legacy
// /ProcSet array without containing a single image.)
func TestRenderedDocumentIsVectorOnly(t *testing.T) {
	raw := render(t, sampleReport(), "en")
	for _, marker := range []string{"/Subtype /Image", "/Subtype/Image"} {
		if bytes.Contains(raw, []byte(marker)) {
			t.Errorf("PDF defines an image object (%q); the trace must be vector drawing commands", marker)
		}
	}

	content := renderText(t, sampleReport(), "en")
	if loc := paintXObject.FindString(content); loc != "" {
		t.Errorf("content stream paints an XObject (%q); the trace must be vector drawing commands", strings.TrimSpace(loc))
	}
}

// paintXObject matches the "/Name Do" operator that paints an XObject. Anchored
// on the leading name so the "Do" in a text string such as "(DOE John) Tj"
// cannot match.
var paintXObject = regexp.MustCompile(`/[A-Za-z0-9_.\-]+\s+Do[\s\]>]`)

func TestDocumentStatesAcquisitionDateTime(t *testing.T) {
	text := renderText(t, sampleReport(), "en")
	if !strings.Contains(text, "2026-09-12 14:32") {
		t.Error("acquisition date and time absent from the page; PDF metadata does not survive printing")
	}
}

// A parameter the source file did not carry must be reported as missing, never
// omitted: a blank bandwidth reads as "unfiltered", which is a different claim.
func TestMissingParametersAreStatedNotOmitted(t *testing.T) {
	r := sampleReport()
	r.Filter = ""
	r.RecordingAt = time.Time{}
	text := renderText(t, r, "en")

	if !strings.Contains(text, "not stated in the source file") {
		t.Error("absent bandwidth is silently omitted instead of being stated")
	}
	if !strings.Contains(text, "Filter:") {
		t.Error("filter label disappears when the value is unknown")
	}
}

func TestBandwidthIsStatedWhenKnown(t *testing.T) {
	text := renderText(t, sampleReport(), "en")
	if !strings.Contains(text, "0.05") || !strings.Contains(text, "150 Hz") {
		t.Errorf("acquisition bandwidth missing from the document:\n%s", text)
	}
}

// Interpretive statements belong to the acquiring device. The document must say
// so, and must not let a reader assume a confirmation status the file never
// carried.
func TestInterpretationIsAttributedAndStatusStated(t *testing.T) {
	text := renderText(t, sampleReport(), "en")
	if !strings.Contains(text, "Interpretation produced by device:") {
		t.Error("interpretive statements are not attributed to the acquiring device")
	}
	if !strings.Contains(text, "SYNTH-CART 1000") {
		t.Error("device model absent from the interpretation attribution")
	}
	if !strings.Contains(text, "Interpretation status:") {
		t.Error("interpretation status absent (IHE CARD TF-2 §4.6.4.2.2)")
	}
	if !strings.Contains(text, "not stated in the source file") {
		t.Error("unknown interpretation status must be stated, not defaulted to confirmed")
	}
}

func TestInterpretationStatusIsReproducedNotInvented(t *testing.T) {
	r := sampleReport()
	r.InterpretationStatus = "Unconfirmed Report"
	text := renderText(t, r, "en")
	if !strings.Contains(text, "Unconfirmed Report") {
		t.Error("status carried by the source file is not reproduced")
	}
}

func TestNoInterpretationBlockWithoutStatements(t *testing.T) {
	r := sampleReport()
	r.Statements = nil
	text := renderText(t, r, "en")
	if strings.Contains(text, "Interpretation status:") {
		t.Error("interpretation status rendered although the file carries no statement")
	}
}

// The renderer reproduces the device's measurements; it derives none of its
// own. RV5+SV1 is the Sokolow-Lyon voltage criterion — computing it would make
// this module produce diagnostic information rather than reproduce it.
func TestNoMeasurementIsDerived(t *testing.T) {
	r := sampleReport()
	r.ShowAmplitudes = true
	r.V5RAmplitude, r.V1SAmplitude = 1.500, 0.600
	text := renderText(t, r, "en")

	if !strings.Contains(text, "1.500") || !strings.Contains(text, "0.600") {
		t.Fatalf("device-reported amplitudes missing:\n%s", text)
	}
	if strings.Contains(text, "2.100") {
		t.Error("RV5+SV1 sum is computed and rendered; no measurement may be derived here")
	}
}

// --- helpers --------------------------------------------------------------

func sampleReport() *Report {
	const n = 500
	lead := make([]int32, n)
	for i := range lead {
		lead[i] = int32(i % 50)
	}
	leads := map[string][]int32{}
	for _, name := range []string{"I", "II", "III", "aVR", "aVL", "aVF", "V1", "V2", "V3", "V4", "V5", "V6"} {
		leads[name] = lead
	}
	return &Report{
		PatientID:   "TEST-1",
		Name:        "DOE John",
		DeviceModel: "SYNTH-CART 1000",
		RecordingAt: time.Date(2026, 9, 12, 14, 32, 0, 0, time.UTC),
		Filter:      "0.05-150 Hz",
		SampleRate:  500,
		ScaleUV:     5,
		Leads:       leads,
		Statements:  []Statement{{Text: "Sinus rhythm", Emphasis: true}},
	}
}

func render(t *testing.T, r *Report, lang string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Render(r, lang, &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.Bytes()
}

// renderText returns the page's drawable text. fpdf deflates its content
// streams, so every stream is inflated and concatenated; the text operands are
// then readable as literal strings.
func renderText(t *testing.T, r *Report, lang string) string {
	t.Helper()
	var out strings.Builder
	raw := render(t, r, lang)

	for rest := raw; ; {
		i := bytes.Index(rest, []byte("stream"))
		if i < 0 {
			break
		}
		body := rest[i+len("stream"):]
		body = bytes.TrimLeft(body, "\r\n")
		j := bytes.Index(body, []byte("endstream"))
		if j < 0 {
			break
		}
		if zr, err := zlib.NewReader(bytes.NewReader(body[:j])); err == nil {
			if dec, err := io.ReadAll(zr); err == nil {
				out.Write(dec)
			}
			zr.Close()
		}
		rest = body[j:]
	}
	if out.Len() == 0 {
		t.Fatal("no content stream could be inflated from the PDF")
	}
	return out.String()
}

// A device that emits a long interpretation must not push its statements over
// the physician block: they are tightened, never dropped and never overlapped.
func TestLongInterpretationStaysInsideItsColumn(t *testing.T) {
	r := sampleReport()
	r.Statements = nil
	for i := 0; i < 12; i++ {
		r.Statements = append(r.Statements, Statement{Text: statementMarkers[i]})
	}
	text := renderText(t, r, "en")
	for _, m := range statementMarkers {
		if !strings.Contains(text, m) {
			t.Errorf("statement %q dropped from a long interpretation list", m)
		}
	}
}

var statementMarkers = []string{
	"ZZA", "ZZB", "ZZC", "ZZD", "ZZE", "ZZF",
	"ZZG", "ZZH", "ZZI", "ZZJ", "ZZK", "ZZL",
}

// --- refusal rather than assumption ---------------------------------------

// Without a gain the amplitude axis has no unit; without a sampling rate the
// time axis has no base. Both used to fall back to a plausible constant, which
// produced a fully calibrated-looking trace at an invented scale.
func TestRenderRefusesWithoutAcquisitionParameters(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(*Report)
	}{
		{"no amplitude scale", func(r *Report) { r.ScaleUV = 0 }},
		{"negative amplitude scale", func(r *Report) { r.ScaleUV = -5 }},
		{"no sampling rate", func(r *Report) { r.SampleRate = 0 }},
		{"negative sampling rate", func(r *Report) { r.SampleRate = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := sampleReport()
			tc.mut(r)
			var buf bytes.Buffer
			err := Render(r, "en", &buf)
			if !errors.Is(err, ErrMissingAcquisitionParameter) {
				t.Fatalf("Render error = %v, want ErrMissingAcquisitionParameter", err)
			}
			if buf.Len() > 0 {
				t.Error("a PDF was produced despite the refusal")
			}
		})
	}
}

// A measurement the device did not report must read as absent, never as zero:
// "0 bpm" and "PR 0 ms" are findings, not blanks.
func TestAbsentMeasurementsRenderAsDash(t *testing.T) {
	r := sampleReport() // all measurement fields left nil
	text := renderText(t, r, "en")

	for _, label := range []string{"Ventricular rate:", "PR interval:", "QRS duration:"} {
		if !strings.Contains(text, label) {
			t.Errorf("measurement row %q missing", label)
		}
	}
	if strings.Contains(text, "(0)") {
		t.Error("an unreported measurement is rendered as 0")
	}
}

func TestReportedMeasurementsAreRendered(t *testing.T) {
	r := sampleReport()
	r.HeartRate = Measured(62)
	r.PRInterval = Measured(154)
	text := renderText(t, r, "en")
	if !strings.Contains(text, "62") || !strings.Contains(text, "154") {
		t.Errorf("reported measurements missing from the table:\n%s", text)
	}
}

func TestMeasuredNonZeroTreatsZeroAsAbsent(t *testing.T) {
	if got := MeasuredNonZero(0); got != nil {
		t.Errorf("MeasuredNonZero(0) = %v, want nil", got)
	}
	if got := MeasuredNonZero(72); got == nil || *got != 72 {
		t.Errorf("MeasuredNonZero(72) = %v, want 72", got)
	}
}

// The footer must not contradict the project's declared intended use. It used
// to say the document was "not for diagnostic use" while the filing described
// the same PDF as intended for reading by a health professional — a
// contradiction a reader resolves against whichever they happen to see.
func TestDisclaimerMatchesTheDeclaredIntendedUse(t *testing.T) {
	text := renderText(t, sampleReport(), "en")

	for _, want := range []string{
		"no measurement and no interpretation",
		"Does not replace the acquisition device",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("footer does not carry %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Not for diagnostic use") {
		t.Error("footer still carries the wording that contradicts the declared intended use")
	}
}

// --- identity provenance ---------------------------------------------------

// An identity that reached the document from the acquisition device, and was
// never confirmed against the HIS, must say so. Both cases render the same
// patient block otherwise, so without this a reader cannot tell a confirmed
// identity from an unconfirmed one.
func TestUnverifiedIdentityIsMarkedOnTheDocument(t *testing.T) {
	r := sampleReport()
	r.IdentityUnverified = true
	text := renderText(t, r, "en")

	if !strings.Contains(text, "IDENTITY NOT CONFIRMED") {
		t.Errorf("unverified identity carries no notice:\n%s", text)
	}
	if !strings.Contains(text, "not verified against the hospital information system") {
		t.Error("the notice does not say what was not verified")
	}
	// The trace must still be produced: withholding it would be worst exactly
	// when the HIS is unreachable and someone needs to read the ECG.
	if !strings.Contains(text, "DOE John") {
		t.Error("the identity itself is no longer rendered")
	}
}

func TestConfirmedIdentityCarriesNoNotice(t *testing.T) {
	text := renderText(t, sampleReport(), "en") // IdentityUnverified defaults to false
	if strings.Contains(text, "IDENTITY NOT CONFIRMED") {
		t.Error("a confirmed identity is marked as unconfirmed")
	}
}

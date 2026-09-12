// Package ecgpdf renders a vendor-neutral 12-lead ECG report (NK paper style)
// to a vector PDF: selectable-text metadata + millimetric red grid + vector
// waveform traces. Vendor front-ends (nk-to-pdf, philips-to-pdf) parse their
// proprietary format and map it into a Report; this package knows nothing about
// NK or Philips.
package ecgpdf

import "time"

// Statement is one interpretive finding. Code is optional (NK carries a 4-digit
// code; Philips does not). Emphasis renders the line in bold (overall banner).
type Statement struct {
	Code     string
	Text     string
	Emphasis bool
}

// Report is the vendor-neutral input to Render.
type Report struct {
	// Identity
	PatientID     string
	Name          string // display name, e.g. "DOE John"
	Sex           string
	BirthDate     string // YYYYMMDD or ""
	Age           string // free text, e.g. "35"
	Height        string // cm, free text
	Weight        string // kg, free text
	BloodPressure string // "systolic/diastolic" in mmHg, free text

	// Clinical context (free text, may be empty)
	Medications []string
	History     string
	Symptoms    string

	// Acquisition context
	DeviceModel string
	Department  string
	Operator    string
	Location    string
	RecordingAt time.Time

	// Measurements produced by the acquiring device, in bpm / ms / degrees.
	//
	// These are pointers so that "absent" is representable. A plain int cannot
	// distinguish a QRS axis the device measured at 0° from one it never
	// reported, and the renderer would print both as "0" — asserting a
	// measurement the source file never made. nil renders as an em dash.
	HeartRate, PRInterval, QRSDuration, QTInterval, QTcInterval *int
	PAxis, QRSAxis, TAxis                                       *int

	// Vendor-specific amplitudes (NK RV5/SV1), read from the source file and
	// reproduced as-is; rendered only when true. No value is ever derived from
	// them — in particular their sum (a voltage criterion) is not computed here.
	ShowAmplitudes             bool
	V5RAmplitude, V1SAmplitude float64

	// Filter spec value, e.g. "H50–150 Hz". The localized "Filter:" word is
	// added by the renderer. An empty value is NOT hidden: the renderer states
	// that the source file does not carry the acquisition bandwidth, so the
	// document never stays silent about a parameter it could not read.
	Filter string

	// Signal
	SampleRate float64            // Hz
	ScaleUV    float64            // µV per sample unit (digit/LSB)
	Leads      map[string][]int32 // expects I,II,III,aVR,aVL,aVF,V1..V6

	// Interpretation. Statements are reproduced verbatim from the source file
	// and attributed to DeviceModel when rendered.
	//
	// InterpretationStatus is the confirmation status the source file carries
	// for those statements ("Confirmed" / "Unconfirmed Report", IHE CARD TF-2
	// §4.6.4.2.2). It is never inferred: when the source does not state it, the
	// renderer says so explicitly rather than picking a default.
	Statements           []Statement
	InterpretationStatus string
}

// Measured wraps a measurement the source file carried, for the optional
// fields of Report.
func Measured(v int) *int { return &v }

// MeasuredNonZero wraps v unless it is zero.
//
// Every vendor model in this repository stores measurements as plain integers
// with zero meaning "the device did not report this", so that is the rule the
// front-ends apply. It is lossy in one direction: a frontal axis the device
// genuinely measured at 0° is reported as absent. That is the safe direction —
// an axis is readable from the trace, whereas a fabricated "0" in the
// measurement table is indistinguishable from a finding. A front-end whose
// format can tell the two apart should call Measured directly.
func MeasuredNonZero(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

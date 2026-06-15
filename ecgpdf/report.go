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
	PatientID string
	Name      string // display name, e.g. "DOE John"
	Sex       string
	BirthDate string // YYYYMMDD or ""
	Age       string // free text, e.g. "35"
	Height    string // cm, free text
	Weight    string // kg, free text

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

	// Measurements (0 = not set, rendered as 0)
	HeartRate, PRInterval, QRSDuration, QTInterval, QTcInterval int
	PAxis, QRSAxis, TAxis                                       int

	// Vendor-specific amplitudes (NK RV5/SV1); rendered only when true.
	ShowAmplitudes             bool
	V5RAmplitude, V1SAmplitude float64

	// Filter spec value, e.g. "H50–150 Hz". The localized "Filter:" word is
	// added by the renderer; an empty value hides the filter part entirely.
	Filter string

	// Signal
	SampleRate float64            // Hz
	ScaleUV    float64            // µV per sample unit (digit/LSB)
	Leads      map[string][]int32 // expects I,II,III,aVR,aVL,aVF,V1..V6

	// Interpretation
	Statements []Statement

	// Forms, when true, renders the patient-identity and measurement values as
	// fillable AcroForm text fields (pre-filled, empty when unknown) so a
	// clinician can complete/correct them and sign in a PDF viewer.
	Forms bool
}

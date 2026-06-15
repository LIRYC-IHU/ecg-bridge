package nktofda

import (
	"strings"
	"time"

	"converter-fda/metaject"
)

// NKData holds all data extracted from a NK .DAT file.
type NKData struct {
	Patient     PatientData
	Measurement MeasurementData
	Record      RecordParams
	Statements  []Statement        // interpretive ECG statements, in file order
	Leads       map[string][]int32 // 8 measured leads: I, II, V1-V6
}

// Statement is one interpretive ECG finding.
type Statement struct {
	Code    string // 4-digit decimal statement code
	Overall bool   // true for the overall assessment banner, false for a specific finding
}

// Anonymize blanks the direct patient identifiers (name, ID, birth date)
// while keeping clinically useful fields (sex, location, recording date,
// device, measurements).
func (d *NKData) Anonymize() {
	d.Patient.FamilyName = ""
	d.Patient.GivenName = ""
	d.Patient.PatientID = ""
	d.Patient.BirthDate = ""
}

// ApplyMetadata overwrites patient-identity and recording-date fields from ov.
// Only fields present in ov are applied; nil fields leave the parsed value.
// A combined PatientName ("LAST^FIRST") is split into family/given; explicit
// familyName/givenName fields take precedence over the combined form.
func (d *NKData) ApplyMetadata(ov *metaject.Override) {
	if ov == nil {
		return
	}
	if ov.PatientID != nil {
		d.Patient.PatientID = *ov.PatientID
	}
	if ov.PatientName != nil {
		fam, giv, _ := strings.Cut(*ov.PatientName, "^")
		d.Patient.FamilyName = fam
		d.Patient.GivenName = giv
	}
	if ov.FamilyName != nil {
		d.Patient.FamilyName = *ov.FamilyName
	}
	if ov.GivenName != nil {
		d.Patient.GivenName = *ov.GivenName
	}
	if ov.Gender != nil {
		d.Patient.Gender = *ov.Gender
	}
	if ov.BirthDate != nil {
		d.Patient.BirthDate = *ov.BirthDate
	}
	if ov.Datetime != nil {
		if t, ok := metaject.ParseDatetime(*ov.Datetime); ok {
			d.Patient.RecordingAt = t
		}
	}
}

// PatientData holds demographic data from PATIENT + PATIENT2 sections.
type PatientData struct {
	FamilyName  string
	GivenName   string
	PatientID   string
	Location    string    // hospital ward/department
	RecordingAt time.Time // datetime from PATIENT section
	Gender      string    // "M", "F", "U", ""
	BirthDate   string    // YYYYMMDD if known, else ""
	DeviceModel string    // from SYSTEM section (e.g. "2350K")

	// Clinical context entered at acquisition (free text, may be empty).
	Age         string   // age in years
	Height      string   // height in cm
	Weight      string   // weight in kg
	Medications []string // current medications
	History     string   // clinical history
	Symptoms    string   // presenting symptoms
}

// MeasurementData holds analytical ECG measurements from MEASUREMENT section.
type MeasurementData struct {
	HeartRate    int     // bpm, 0 = not set
	PRInterval   int     // ms
	QRSDuration  int     // ms
	QTInterval   int     // ms
	QTcInterval  int     // ms
	PAxis        int     // deg, includes sign
	QRSAxis      int     // deg, includes sign
	TAxis        int     // deg, includes sign
	HasPAxis     bool
	HasQRSAxis   bool
	HasTAxis     bool
	V5RAmplitude float64 // µV
	V1SAmplitude float64 // µV
}

// RecordParams holds waveform recording parameters from RECORD section.
type RecordParams struct {
	SampleRate   int
	TotalSamples int
	Scale        float64 // µV/digit, always 1.25 for NK type 16
}

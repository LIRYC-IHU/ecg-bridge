package fukudatofda

import (
	"strings"
	"time"

	"github.com/LIRYC-IHU/ecg-bridge/metaject"
)

// FukudaData holds everything extracted from a Fukuda .ECG file.
type FukudaData struct {
	Patient     PatientData
	Record      RecordParams
	Measurement MeasurementData
	Statements  []Statement        // interpretive ECG statements, in file order
	Leads       map[string][]int32 // 8 measured leads: I, II, V1-V6
}

// MeasurementData holds the analytical ECG measurements from the header.
type MeasurementData struct {
	HeartRate   int // bpm, 0 = not set
	PRInterval  int // ms
	QRSDuration int // ms
	QTInterval  int // ms
	QTcInterval int // ms
	QRSAxis     int // deg
	HasQRSAxis  bool
}

// Statement is one interpretive ECG finding (Fukuda INTER12L code).
type Statement struct {
	Code string // 4-digit Fukuda statement code (e.g. "8716")
}

// PatientData holds demographics and acquisition context.
type PatientData struct {
	FamilyName  string
	GivenName   string
	PatientID   string
	Location    string
	RecordingAt time.Time
	Gender      string // "M", "F", "U", ""
	BirthDate   string // YYYYMMDD if known, else ""
	DeviceModel string // e.g. "FX-8322"
}

// RecordParams holds waveform recording parameters from the .ECG header.
type RecordParams struct {
	SampleRate   int
	TotalSamples int
	NumLeads     int
	Scale        float64 // µV/digit (AVM / 1000), typically 4.88
}

// Anonymize blanks the direct patient identifiers.
func (d *FukudaData) Anonymize() {
	d.Patient.FamilyName = ""
	d.Patient.GivenName = ""
	d.Patient.PatientID = ""
	d.Patient.BirthDate = ""
}

// ApplyMetadata overwrites patient-identity and recording-date fields from ov.
// Only fields present in ov are applied; nil fields leave the parsed value.
func (d *FukudaData) ApplyMetadata(ov *metaject.Override) {
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

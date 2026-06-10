package nktofda

import "time"

// NKData holds all data extracted from a NK .DAT file.
type NKData struct {
	Patient     PatientData
	Measurement MeasurementData
	Record      RecordParams
	Leads       map[string][]int32 // 8 measured leads: I, II, V1-V6
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

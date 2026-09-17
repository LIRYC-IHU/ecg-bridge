package mindraytofda

import (
	"time"

	"github.com/LIRYC-IHU/ecg-bridge/metaject"
)

type MindrayData struct {
	Patient     PatientData
	Device      DeviceData
	Record      RecordParams
	Leads       map[string][]int
	Measurement MeasurementData
}

type PatientData struct {
	Name      string
	PatientID string
	Gender    string // "M", "F", "UN"
	// BirthDate is never present in a Mindray file. It exists so an establishment's
	// date of birth can be injected, which is what keeps the rendered report's
	// identity internally consistent — a name from the HIS above an age from the
	// acquisition device is the mismatch a clinician cross-checks on.
	BirthDate string // YYYYMMDD, or "" when unknown
	Paced     bool
	Location  string
	StartTime time.Time
	EndTime   time.Time
}

// Anonymize blanks the direct patient identifiers (name, ID) while keeping
// clinically useful fields (sex, location, acquisition dates, measurements).
// Mindray carries no birth date field.
func (d *MindrayData) Anonymize() {
	d.Patient.Name = ""
	d.Patient.PatientID = ""
	// A date of birth identifies; an injected one must not survive anonymisation.
	d.Patient.BirthDate = ""
}

// ApplyMetadata overwrites patient-identity and acquisition-date fields from ov.
// Only fields present in ov are applied; nil fields leave the parsed value.
func (d *MindrayData) ApplyMetadata(ov *metaject.Override) {
	if ov == nil {
		return
	}
	if ov.PatientID != nil {
		d.Patient.PatientID = *ov.PatientID
	}
	if ov.PatientName != nil {
		d.Patient.Name = *ov.PatientName
	}
	if ov.Gender != nil {
		d.Patient.Gender = *ov.Gender
	}
	if ov.BirthDate != nil {
		d.Patient.BirthDate = *ov.BirthDate
	}
	if ov.Datetime != nil {
		if t, ok := metaject.ParseDatetime(*ov.Datetime); ok {
			d.Patient.StartTime = t
		}
	}
}

type DeviceData struct {
	SerialNumber string
	SoftwareName string
	ModelName    string
	Manufacturer string
}

type RecordParams struct {
	SampleRate int
	Scale      float64 // µV per digit (1.0 for Mindray — data is already µV)
}

type MeasurementData struct {
	HeartRate   int
	PRInterval  int
	QRSDuration int
	QTInterval  int
	QTcInterval int
	PAxis       int
	QRSAxis     int
	TAxis       int
	HasPAxis    bool
	HasQRSAxis  bool
	HasTAxis    bool
}

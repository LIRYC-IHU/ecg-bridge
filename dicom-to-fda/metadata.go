package dicomtofda

import (
	"github.com/LIRYC-IHU/ecg-bridge/metaject"
)

// PatientMetadata holds the patient/study fields extracted from the DICOM,
// and serves as the internal holder applied to the FDA aECG output.
type PatientMetadata struct {
	// Patient
	PatientName      string
	PatientID        string
	SecondPatientID  string
	PatientBirthDate string // YYYYMMDD
	PatientSex       string // M, F, UN, O
	PatientAge       string
	Paced            string // "true" / "false"

	// Clinical context
	Bed                     string
	Room                    string
	PointOfCare             string
	Medications             []string
	ClinicalClassifications []string

	// Site / trial
	LocationName     string // trialSite location name
	InvestigatorName string // referring physician / investigator

	// Device / acquisition
	DeviceModel     string
	DeviceSerial    string
	SoftwareVersion string
	Manufacturer    string
	OperatorsName   string
	InstitutionName string
	StudyDate       string // YYYYMMDD
	StudyTime       string // HHMMSS
}

// Anonymize blanks the direct patient identifiers (name, IDs, birth date)
// while keeping clinically useful fields (sex, age, acquisition dates,
// measurements).
func (d *DicomData) Anonymize() {
	d.Patient.PatientName = ""
	d.Patient.PatientID = ""
	d.Patient.SecondPatientID = ""
	d.Patient.PatientBirthDate = ""
}

// ApplyMetadata overwrites patient-identity and study-date fields from ov.
// Only fields present in ov are applied; nil fields leave the parsed value.
func (d *DicomData) ApplyMetadata(ov *metaject.Override) {
	if ov == nil {
		return
	}
	if ov.PatientID != nil {
		d.Patient.PatientID = *ov.PatientID
	}
	if ov.PatientName != nil {
		d.Patient.PatientName = *ov.PatientName
	}
	if ov.Gender != nil {
		d.Patient.PatientSex = *ov.Gender
	}
	if ov.Age != nil {
		d.Patient.PatientAge = *ov.Age
	}
	if ov.BirthDate != nil {
		d.Patient.PatientBirthDate = *ov.BirthDate
	}
	if ov.Datetime != nil {
		d.StudyDate, d.StudyTime = metaject.SplitDatetime(*ov.Datetime)
	}
}

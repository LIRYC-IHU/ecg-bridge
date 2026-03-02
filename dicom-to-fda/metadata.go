package dicomtofda

import (
	"encoding/json"
	"os"
)

// PatientMetadata holds optional patient overrides from a JSON file.
// All fields are optional; zero values mean "not provided".
type PatientMetadata struct {
	// Patient
	PatientName      string `json:"patientName"`
	PatientID        string `json:"patientID"`
	SecondPatientID  string `json:"secondPatientID"`
	PatientBirthDate string `json:"patientBirthDate"` // YYYYMMDD
	PatientSex       string `json:"patientSex"`       // M, F, UN, O
	PatientAge       string `json:"patientAge"`
	Paced            string `json:"paced"` // "true" / "false"

	// Clinical context
	Bed                     string   `json:"bed"`
	Room                    string   `json:"room"`
	PointOfCare             string   `json:"pointOfCare"`
	Medications             []string `json:"medications"`
	ClinicalClassifications []string `json:"clinicalClassifications"`

	// Site / trial
	LocationName    string `json:"locationName"`    // trialSite location name
	InvestigatorName string `json:"investigatorName"` // referring physician / investigator

	// Device / acquisition
	DeviceModel     string `json:"deviceModel"`
	DeviceSerial    string `json:"deviceSerial"`
	SoftwareVersion string `json:"softwareVersion"`
	Manufacturer    string `json:"manufacturer"`
	OperatorsName   string `json:"operatorsName"`
	InstitutionName string `json:"institutionName"`
	StudyDate       string `json:"studyDate"` // YYYYMMDD
	StudyTime       string `json:"studyTime"` // HHMMSS
}

// LoadMetadata reads a patient JSON file and returns a PatientMetadata struct.
func LoadMetadata(path string) (*PatientMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m PatientMetadata
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

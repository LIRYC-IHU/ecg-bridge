package musetofda

import "encoding/xml"

// MuseData is the intermediate representation parsed from a GE MUSE
// RestingECG XML file, ready to be converted to FDA aECG XML.
type MuseData struct {
	// Device
	MuseVersion string

	// Study / acquisition (often empty in anonymized exports)
	StudyDate string // YYYYMMDD
	StudyTime string // HHMMSS
	StudyUID  string // generated OID/UUID

	// Patient (often empty in anonymized exports)
	PatientID   string
	PatientName string // "LAST^FIRST"
	PatientSex  string // "M" / "F" / ""
	PatientAge  string // e.g. "050Y"

	// Signal characteristics
	SamplingRate float64 // Hz (SampleBase)
	Sensitivity  float64 // µV per LSB (LeadAmplitudeUnitsPerBit)
	Baseline     float64 // FirstSampleBaseline

	// Filters (from the Rhythm waveform header)
	FilterHPF   float64 // high-pass cutoff
	FilterLPF   float64 // low-pass cutoff
	NotchFilter float64 // AC filter

	// Rhythm waveform (ORIGINAL) — 12 leads, 4 of them derived
	RhythmLeads [12][]int16
	// Median waveform (DERIVED, representative beat) — 12 leads
	MedianLeads [12][]int16

	// Global measurements
	HeartRate    float64 // VentricularRate
	AtrialRate   float64
	PRInterval   float64
	QRSDuration  float64
	QTInterval   float64
	QTcInterval  float64 // QTCorrected
	PFrontAxis   float64 // PAxis
	QRSFrontAxis float64 // RAxis
	TFrontAxis   float64 // TAxis

	// Diagnosis / interpretation statements
	DiagnosisStatements []string
}

// --- Raw XML structs (GE MUSE RestingECG, DTD restecg.dtd) ---

type museXML struct {
	XMLName      xml.Name         `xml:"RestingECG"`
	MuseInfo     museInfo         `xml:"MuseInfo"`
	Patient      musePatient      `xml:"PatientDemographics"`
	Test         museTest         `xml:"TestDemographics"`
	Measurements museMeasurements `xml:"RestingECGMeasurements"`
	Diagnosis    museDiagnosis    `xml:"Diagnosis"`
	Waveforms    []museWaveform   `xml:"Waveform"`
}

type museInfo struct {
	Version string `xml:"MuseVersion"`
}

// musePatient mirrors the standard MUSE PatientDemographics block.
// Absent in anonymized exports; all fields default to "".
type musePatient struct {
	PatientID string `xml:"PatientID"`
	LastName  string `xml:"PatientLastName"`
	FirstName string `xml:"PatientFirstName"`
	Gender    string `xml:"Gender"` // MALE / FEMALE / UNKNOWN
	Age       string `xml:"PatientAge"`
	AgeUnits  string `xml:"AgeUnits"`
}

// museTest mirrors the standard MUSE TestDemographics block.
type museTest struct {
	AcquisitionDate string `xml:"AcquisitionDate"` // MM-DD-YYYY
	AcquisitionTime string `xml:"AcquisitionTime"` // HH:MM:SS
}

type museMeasurements struct {
	VentricularRate string `xml:"VentricularRate"`
	AtrialRate      string `xml:"AtrialRate"`
	PRInterval      string `xml:"PRInterval"`
	QRSDuration     string `xml:"QRSDuration"`
	QTInterval      string `xml:"QTInterval"`
	QTCorrected     string `xml:"QTCorrected"`
	PAxis           string `xml:"PAxis"`
	RAxis           string `xml:"RAxis"`
	TAxis           string `xml:"TAxis"`
}

type museDiagnosis struct {
	Statements []museStmt `xml:"DiagnosisStatement"`
}

type museStmt struct {
	Flag string `xml:"StmtFlag"`
	Text string `xml:"StmtText"`
}

type museWaveform struct {
	Type           string         `xml:"WaveformType"` // Median / Rhythm
	SampleBase     string         `xml:"SampleBase"`
	HighPassFilter string         `xml:"HighPassFilter"`
	LowPassFilter  string         `xml:"LowPassFilter"`
	ACFilter       string         `xml:"ACFilter"`
	Leads          []museLeadData `xml:"LeadData"`
}

type museLeadData struct {
	ID                   string `xml:"LeadID"`
	AmplitudeUnitsPerBit string `xml:"LeadAmplitudeUnitsPerBit"`
	SampleCountTotal     string `xml:"LeadSampleCountTotal"`
	FirstSampleBaseline  string `xml:"FirstSampleBaseline"`
	Data                 string `xml:"WaveFormData"` // base64, int16 little-endian
}

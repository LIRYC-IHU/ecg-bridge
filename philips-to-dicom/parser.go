package philipstodicom

import (
	"encoding/xml"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// PhilipsData is the unified model extracted from a Philips SierraECG XML file.
type PhilipsData struct {
	// Patient
	PatientID   string
	PatientName string // "LASTNAME^FIRSTNAME"
	PatientSex  string // "M" / "F" / ""
	PatientAge  string // "050Y"

	// Study / acquisition
	StudyDate string // YYYYMMDD
	StudyTime string // HHMMSS
	StudyUID  string // generated OID

	// Device
	Manufacturer    string
	ModelName       string
	SoftwareVer     string
	InstitutionName string
	OperatorID      string
	Room            string

	// Signal
	SamplingRate  float64
	BitsPerSample int
	NumChannels   int
	Sensitivity   float64 // µV/LSB
	Baseline      float64

	// Filters
	FilterHPF   float64 // high-pass cutoff (Hz)
	FilterLPF   float64 // low-pass cutoff (Hz)
	NotchFilter float64 // notch (Hz)

	// Waveforms rhythm (ORIGINAL) — 12 leads × 5500 samples
	RhythmLeads [12][]int16

	// Waveforms representative beat (DERIVED) — leadName → samples
	RepBeats map[string][]int16

	// Global measurements
	HeartRate    float64
	PRInterval   float64
	QRSDuration  float64
	QTInterval   float64
	QTcInterval  float64
	AtrialRate   float64
	PFrontAxis   float64 // degrees
	QRSFrontAxis float64 // degrees
	TFrontAxis   float64 // degrees
	QTDispersion float64 // ms
}

const philipsNamespace = "http://www3.medical.philips.com"

// validatePhilipsXML vérifie que le contenu XML est bien un fichier Philips SierraECG.
func validatePhilipsXML(raw []byte) error {
	// Scan rapide des 4 Ko pour éviter de tout parser
	head := raw
	if len(head) > 4096 {
		head = raw[:4096]
	}
	s := string(head)
	if !strings.Contains(s, philipsNamespace) {
		return fmt.Errorf("not a Philips SierraECG XML file (namespace %q not found)", philipsNamespace)
	}
	return nil
}

// ParsePhilips reads a Philips SierraECG XML file and returns a PhilipsData.
func ParsePhilips(path string) (*PhilipsData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	if err := validatePhilipsXML(raw); err != nil {
		return nil, err
	}

	var px PhilipsXML
	if err := xml.Unmarshal(raw, &px); err != nil {
		return nil, fmt.Errorf("parsing XML: %w", err)
	}

	version := px.DocumentInfo.Version
	if !strings.HasPrefix(version, "1.03") && !strings.HasPrefix(version, "1.04") {
		return nil, fmt.Errorf("unsupported SierraECG version: %s", version)
	}

	d := &PhilipsData{}

	// Patient
	pg := px.Patient.General
	d.PatientID = strings.TrimSpace(pg.PatientID)
	d.PatientName = formatPatientName(pg.Name.Last, pg.Name.First)
	d.PatientSex = mapSex(pg.Sex)
	d.PatientAge = formatAge(pg.Age.Default)

	// Study date/time
	d.StudyDate = formatDate(px.DataAcq.Date)
	d.StudyTime = formatTime(px.DataAcq.Time)
	d.StudyUID = fmt.Sprintf("2.25.%d", time.Now().UnixNano())

	// Device
	d.Manufacturer = "Philips Medical Systems"
	d.ModelName, d.SoftwareVer = parseMachineDetail(px.DataAcq.Machine.Detail)
	if d.ModelName == "" {
		d.ModelName = strings.TrimSpace(px.DataAcq.Machine.Name)
	}
	d.InstitutionName = strings.TrimSpace(px.DataAcq.Acquirer.InstitutionName)
	d.OperatorID = strings.TrimSpace(px.DataAcq.Acquirer.OperatorID)
	d.Room = strings.TrimSpace(px.DataAcq.Acquirer.Room)

	// Signal
	d.SamplingRate = parseFloat(px.DataAcq.Signal.SamplingRate)
	d.BitsPerSample = parseInt(px.DataAcq.Signal.BitsPerSample)
	d.NumChannels = parseInt(px.DataAcq.Signal.Channels)
	d.Sensitivity = parseFloat(px.DataAcq.Signal.Resolution)
	if d.Sensitivity == 0 {
		d.Sensitivity = 5.0
	}
	d.Baseline = parseFloat(px.DataAcq.Signal.SignalOffset)

	// Filters: signalbandwidth "0.05-150" → HPF=0.05, LPF=150
	d.FilterHPF, d.FilterLPF = parseBandwidth(px.DataAcq.Signal.Bandwidth)
	notch := strings.TrimSpace(px.DataAcq.Signal.AcSetting)
	if notch == "" {
		notch = strings.TrimSpace(px.ReportInfo.Bandwidth.Notch)
	}
	d.NotchFilter = parseFloat(notch)

	// Rhythm waveforms — XLI decode
	leads, err := decodeRhythmLeads(px.Waveforms.ParsedWaveforms)
	if err != nil {
		return nil, fmt.Errorf("decoding rhythm waveforms: %w", err)
	}
	d.RhythmLeads = leads

	// Representative beat waveforms (plain text integers in XML)
	d.RepBeats = parseRepBeats(px.Waveforms.RepBeats)

	// Global measurements
	d.HeartRate = parseFloat(px.Interpretations.Interpretation.Measurements.HeartRate)
	d.PRInterval = parseFloat(px.Measurements.Global.MeanPR)
	d.QRSDuration = parseFloat(px.Measurements.Global.MeanQRS)
	d.QTInterval = parseFloat(px.Measurements.Global.MeanQT)
	d.QTcInterval = parseFloat(px.Measurements.Global.MeanQTc)
	d.AtrialRate = parseFloat(px.Measurements.Global.AtrialRate)
	d.PFrontAxis = parseFloat(px.Measurements.Global.PFrontAxis)
	d.QRSFrontAxis = parseFloat(px.Measurements.Global.QRSFrontAxis)
	d.TFrontAxis = parseFloat(px.Measurements.Global.TFrontAxis)
	d.QTDispersion = parseFloat(px.Measurements.Global.QTDispersion)

	return d, nil
}

func formatPatientName(last, first string) string {
	last = strings.TrimSpace(last)
	first = strings.TrimSpace(first)
	if last == "" && first == "" {
		return ""
	}
	return last + "^" + first
}

func mapSex(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "male":
		return "M"
	case "female":
		return "F"
	default:
		return ""
	}
}

func formatAge(defaultAge string) string {
	defaultAge = strings.TrimSpace(defaultAge)
	if defaultAge == "" {
		return ""
	}
	n, err := strconv.Atoi(defaultAge)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%03dY", n)
}

func formatDate(date string) string {
	return strings.ReplaceAll(strings.TrimSpace(date), "-", "")
}

func formatTime(t string) string {
	return strings.ReplaceAll(strings.TrimSpace(t), ":", "")
}

func parseMachineDetail(detail string) (model, software string) {
	// "Philips Medical Products:860306:A.07.07.07"
	parts := strings.Split(detail, ":")
	if len(parts) >= 3 {
		return strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
	}
	if len(parts) >= 2 {
		return strings.TrimSpace(parts[1]), ""
	}
	return "", ""
}

func parseBandwidth(bw string) (hpf, lpf float64) {
	// "0.05-150" → hpf=0.05, lpf=150
	parts := strings.SplitN(strings.TrimSpace(bw), "-", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	return parseFloat(parts[0]), parseFloat(parts[1])
}

// leadOrder is the canonical DICOM 12-lead order.
var leadOrder = []string{"I", "II", "III", "aVR", "aVL", "aVF", "V1", "V2", "V3", "V4", "V5", "V6"}

func parseRepBeats(rb RepBeats) map[string][]int16 {
	result := make(map[string][]int16)
	for _, repbeat := range rb.RepBeat {
		name := strings.TrimSpace(repbeat.LeadName)
		if name == "" {
			continue
		}
		data := strings.TrimSpace(repbeat.Data)
		if data == "" {
			result[name] = []int16{}
			continue
		}
		fields := strings.Fields(data)
		samples := make([]int16, 0, len(fields))
		for _, f := range fields {
			n, err := strconv.ParseInt(f, 10, 32)
			if err == nil {
				samples = append(samples, int16(n))
			}
		}
		result[name] = samples
	}
	return result
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

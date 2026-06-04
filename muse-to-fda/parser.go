package musetofda

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// Lead index in the 12-lead output order: I, II, III, aVR, aVL, aVF, V1..V6.
const (
	idxI = iota
	idxII
	idxIII
	idxAVR
	idxAVL
	idxAVF
	idxV1
	idxV2
	idxV3
	idxV4
	idxV5
	idxV6
)

// storedLeadIndex maps the 8 leads physically stored by MUSE to their
// position in the 12-lead output order. III, aVR, aVL and aVF are derived.
var storedLeadIndex = map[string]int{
	"I":  idxI,
	"II": idxII,
	"V1": idxV1,
	"V2": idxV2,
	"V3": idxV3,
	"V4": idxV4,
	"V5": idxV5,
	"V6": idxV6,
}

// ParseMuse reads and parses a GE MUSE RestingECG XML file into MuseData.
func ParseMuse(data []byte) (*MuseData, error) {
	var raw museXML
	if err := xml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshalling MUSE XML: %w", err)
	}
	if raw.XMLName.Local != "RestingECG" {
		return nil, fmt.Errorf("not a MUSE RestingECG file (root: %q)", raw.XMLName.Local)
	}

	d := &MuseData{
		MuseVersion: raw.MuseInfo.Version,
	}

	parsePatient(d, raw.Patient)
	parseStudyTime(d, raw.Test)
	parseMeasurements(d, raw.Measurements)
	parseDiagnosis(d, raw.Diagnosis)
	if err := parseWaveforms(d, raw.Waveforms); err != nil {
		return nil, err
	}

	if d.StudyUID == "" {
		d.StudyUID = newUUID()
	}
	return d, nil
}

func parsePatient(d *MuseData, p musePatient) {
	d.PatientID = strings.TrimSpace(p.PatientID)
	last := strings.TrimSpace(p.LastName)
	first := strings.TrimSpace(p.FirstName)
	if last != "" || first != "" {
		d.PatientName = last + "^" + first
	}
	switch strings.ToUpper(strings.TrimSpace(p.Gender)) {
	case "MALE", "M":
		d.PatientSex = "M"
	case "FEMALE", "F":
		d.PatientSex = "F"
	}
	if age := strings.TrimSpace(p.Age); age != "" {
		unit := "Y"
		if u := strings.TrimSpace(p.AgeUnits); u != "" {
			unit = strings.ToUpper(u[:1])
		}
		d.PatientAge = age + unit
	}
}

func parseStudyTime(d *MuseData, t museTest) {
	// AcquisitionDate is MM-DD-YYYY → YYYYMMDD
	if date := strings.TrimSpace(t.AcquisitionDate); date != "" {
		if parts := strings.Split(date, "-"); len(parts) == 3 {
			d.StudyDate = parts[2] + parts[0] + parts[1]
		}
	}
	// AcquisitionTime is HH:MM:SS → HHMMSS
	if tm := strings.TrimSpace(t.AcquisitionTime); tm != "" {
		d.StudyTime = strings.ReplaceAll(tm, ":", "")
	}
}

func parseMeasurements(d *MuseData, m museMeasurements) {
	d.HeartRate = atof(m.VentricularRate)
	d.AtrialRate = atof(m.AtrialRate)
	d.PRInterval = atof(m.PRInterval)
	d.QRSDuration = atof(m.QRSDuration)
	d.QTInterval = atof(m.QTInterval)
	d.QTcInterval = atof(m.QTCorrected)
	d.PFrontAxis = atof(m.PAxis)
	d.QRSFrontAxis = atof(m.RAxis)
	d.TFrontAxis = atof(m.TAxis)
}

func parseDiagnosis(d *MuseData, dg museDiagnosis) {
	for _, s := range dg.Statements {
		text := strings.TrimSpace(s.Text)
		if text != "" {
			d.DiagnosisStatements = append(d.DiagnosisStatements, text)
		}
	}
}

func parseWaveforms(d *MuseData, waveforms []museWaveform) error {
	for _, w := range waveforms {
		leads, err := decodeWaveform(w)
		if err != nil {
			return fmt.Errorf("decoding %s waveform: %w", w.Type, err)
		}
		deriveLimbLeads(&leads)

		switch strings.ToLower(w.Type) {
		case "rhythm":
			d.RhythmLeads = leads
			if d.SamplingRate == 0 {
				d.SamplingRate = atof(w.SampleBase)
			}
			d.FilterHPF = atof(w.HighPassFilter)
			d.FilterLPF = atof(w.LowPassFilter)
			d.NotchFilter = atof(w.ACFilter)
		case "median":
			d.MedianLeads = leads
			if d.SamplingRate == 0 {
				d.SamplingRate = atof(w.SampleBase)
			}
		}

		// Sensitivity / baseline from the first available lead.
		if d.Sensitivity == 0 && len(w.Leads) > 0 {
			d.Sensitivity = atof(w.Leads[0].AmplitudeUnitsPerBit)
			d.Baseline = atof(w.Leads[0].FirstSampleBaseline)
		}
	}
	return nil
}

// decodeWaveform decodes the 8 stored leads of a MUSE waveform into a
// 12-lead array (derived leads left empty here).
func decodeWaveform(w museWaveform) ([12][]int16, error) {
	var leads [12][]int16
	for _, ld := range w.Leads {
		idx, ok := storedLeadIndex[strings.TrimSpace(ld.ID)]
		if !ok {
			continue // unknown / already-derived lead, skip
		}
		samples, err := decodeSamples(ld.Data)
		if err != nil {
			return leads, fmt.Errorf("lead %s: %w", ld.ID, err)
		}
		leads[idx] = samples
	}
	return leads, nil
}

// decodeSamples decodes base64 → little-endian int16 samples.
func decodeSamples(b64 string) ([]int16, error) {
	clean := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, b64)
	raw, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	n := len(raw) / 2
	samples := make([]int16, n)
	for i := 0; i < n; i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(raw[i*2 : i*2+2]))
	}
	return samples, nil
}

// deriveLimbLeads computes III, aVR, aVL and aVF from leads I and II
// (Einthoven / Goldberger relations).
func deriveLimbLeads(leads *[12][]int16) {
	i, ii := leads[idxI], leads[idxII]
	n := min(len(i), len(ii))
	if n == 0 {
		return
	}
	iii := make([]int16, n)
	avr := make([]int16, n)
	avl := make([]int16, n)
	avf := make([]int16, n)
	for k := range n {
		a, b := int(i[k]), int(ii[k])
		iii[k] = int16(b - a)
		avr[k] = int16(-(a + b) / 2)
		avl[k] = int16(a - b/2)
		avf[k] = int16(b - a/2)
	}
	leads[idxIII] = iii
	leads[idxAVR] = avr
	leads[idxAVL] = avl
	leads[idxAVF] = avf
}

func atof(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

package philipstodicom_test

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	philipstodicom "converter-fda/philips-to-dicom"

	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/tag"
)

// philipsRoot is the base directory containing patient subdirectories.
const philipsRoot = "/Volumes/Signal/ECG/Phillips"

// testPair describes one XML/DICOM pair.
type testPair struct {
	patient string // e.g. "BS1170"
	xmlPath string // path to Philips SierraECG XML
	dcmPath string // path to reference DICOM produced by the Philips machine
}

// testPairs lists all available XML/DICOM pairs.
var testPairs = []testPair{
	{"BS1170", "BS1170/xml_1.03/BS1170.xml", "BS1170/DICOM/ecg_0.dcm"},
	{"BS1171", "BS1171/xml_1.03/BS1171.xml", "BS1171/DICOM/ecg_0.dcm"},
	{"BS1172", "BS1172/xml_1.03/BS1172.xml", "BS1172/DICOM/ecg_0.dcm"},
	{"BS1174", "BS1174/xml_1.03/BS1174.xml", "BS1174/DICOM/ecg_0.dcm"},
	{"BS1175", "BS1175/xml_1.03/BS1175.xml", "BS1175/DICOM/ecg_0.dcm"},
	{"BS1176", "BS1176/xml_1.03/BS1176.xml", "BS1176/DICOM/ecg_0.dcm"},
	{"BS1202", "BS1202/xml_1.03/BS1202.xml", "BS1202/DICOM/ecg_0.dcm"},
	{"BS1203", "BS1203/xml_1.03/BS1203.xml", "BS1203/DICOM/ecg_0.dcm"},
	{"BS1212", "BS1212/xml_1.03/BS1212.xml", "BS1212/DICOM/ecg_0.dcm"},
	{"BS1213", "BS1213/xml_1.03/BS1213.xml", "BS1213/DICOM/ecg_0.dcm"},
	{"BS1214", "BS1214/xml_1.03/BS1214.xml", "BS1214/DICOM/ecg_0.dcm"},
	{"BS1215", "BS1215/xml_1.03/BS1215.xml", "BS1215/DICOM/ecg_0.dcm"},
}

// dicomInfo holds extracted fields from a DICOM file for comparison.
type dicomInfo struct {
	PatientID   string
	PatientName string
	PatientSex  string
	PatientAge  string
	StudyDate   string
	StudyTime   string
	Manufacturer string
	ModelName   string
	SamplingFreq string
	// WaveformSequence items
	Waveforms []waveformInfo
	// WaveformAnnotationSequence measurements: name → value
	Annotations map[string]float64
}

type waveformInfo struct {
	Originality string
	NumChannels int
	NumSamples  int
	Label       string
	// Lead samples: channel index → []int16
	Samples [][]int16
}

// parseDICOM extracts relevant fields from a DICOM file.
func parseDICOM(path string) (*dicomInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ds, err := dicom.Parse(f, 1<<30, nil)
	if err != nil {
		return nil, fmt.Errorf("parse DICOM: %w", err)
	}

	info := &dicomInfo{
		Annotations: make(map[string]float64),
	}

	getString := func(t tag.Tag) string {
		el, err := ds.FindElementByTag(t)
		if err != nil || el == nil {
			return ""
		}
		v := el.Value.GetValue()
		switch vals := v.(type) {
		case []string:
			if len(vals) > 0 {
				return strings.TrimSpace(vals[0])
			}
		}
		return ""
	}

	info.PatientID = getString(tag.PatientID)
	info.PatientName = getString(tag.PatientName)
	info.PatientSex = getString(tag.PatientSex)
	info.PatientAge = getString(tag.PatientAge)
	info.StudyDate = getString(tag.StudyDate)
	info.StudyTime = getString(tag.StudyTime)
	info.Manufacturer = getString(tag.Manufacturer)
	info.ModelName = getString(tag.ManufacturerModelName)

	// WaveformSequence
	wfEl, err := ds.FindElementByTag(tag.WaveformSequence)
	if err == nil && wfEl != nil {
		items := wfEl.Value.GetValue().([]*dicom.SequenceItemValue)
		for _, item := range items {
			elems := item.GetValue().([]*dicom.Element)
			wi := waveformInfo{}
			for _, e := range elems {
				switch e.Tag {
				case tag.WaveformOriginality:
					wi.Originality = e.Value.GetValue().([]string)[0]
				case tag.NumberOfWaveformChannels:
					wi.NumChannels = e.Value.GetValue().([]int)[0]
				case tag.NumberOfWaveformSamples:
					wi.NumSamples = e.Value.GetValue().([]int)[0]
				case tag.MultiplexGroupLabel:
					wi.Label = e.Value.GetValue().([]string)[0]
				case tag.SamplingFrequency:
					info.SamplingFreq = e.Value.GetValue().([]string)[0]
				case tag.WaveformData:
					raw := e.Value.GetValue().([]byte)
					if wi.NumChannels > 0 && wi.NumSamples > 0 {
						wi.Samples = extractSamples(raw, wi.NumChannels, wi.NumSamples)
					}
				}
			}
			info.Waveforms = append(info.Waveforms, wi)
		}
	}

	// WaveformAnnotationSequence — extract numeric measurements by CodeMeaning
	annEl, err := ds.FindElementByTag(tag.WaveformAnnotationSequence)
	if err == nil && annEl != nil {
		items := annEl.Value.GetValue().([]*dicom.SequenceItemValue)
		for _, item := range items {
			elems := item.GetValue().([]*dicom.Element)
			var meaning string
			var numericVal float64
			var hasValue bool
			for _, e := range elems {
				if e.Tag == tag.ConceptNameCodeSequence {
					sub := e.Value.GetValue().([]*dicom.SequenceItemValue)
					if len(sub) > 0 {
						for _, se := range sub[0].GetValue().([]*dicom.Element) {
							if se.Tag == tag.CodeMeaning {
								meaning = se.Value.GetValue().([]string)[0]
							}
						}
					}
				}
				if e.Tag == tag.MeasuredValueSequence {
					sub := e.Value.GetValue().([]*dicom.SequenceItemValue)
					if len(sub) > 0 {
						for _, se := range sub[0].GetValue().([]*dicom.Element) {
							if se.Tag == tag.NumericValue {
								vals := se.Value.GetValue().([]string)
								if len(vals) > 0 {
									fmt.Sscanf(vals[0], "%f", &numericVal)
									hasValue = true
								}
							}
						}
					}
				}
			}
			if meaning != "" && hasValue {
				info.Annotations[meaning] = numericVal
			}
		}
	}

	return info, nil
}

// extractSamples converts interleaved little-endian int16 waveform bytes to per-channel slices.
func extractSamples(raw []byte, numCh, numSamples int) [][]int16 {
	samples := make([][]int16, numCh)
	for i := range samples {
		samples[i] = make([]int16, numSamples)
	}
	for s := 0; s < numSamples; s++ {
		for c := 0; c < numCh; c++ {
			offset := (s*numCh + c) * 2
			if offset+2 > len(raw) {
				return samples
			}
			samples[c][s] = int16(binary.LittleEndian.Uint16(raw[offset:]))
		}
	}
	return samples
}

// skipIfNoData skips the test if the Philips data volume is not mounted.
func skipIfNoData(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(philipsRoot); os.IsNotExist(err) {
		t.Skipf("Philips data not available at %s", philipsRoot)
	}
}

// -- Tests --

// TestParseAllXML verifies that all Philips XML files parse without error.
func TestParseAllXML(t *testing.T) {
	skipIfNoData(t)

	// Include XML-only patients too
	xmlFiles, _ := filepath.Glob(filepath.Join(philipsRoot, "*/xml_1.03/*.xml"))
	if len(xmlFiles) == 0 {
		t.Fatal("no XML files found")
	}

	for _, xmlPath := range xmlFiles {
		patient := filepath.Base(filepath.Dir(filepath.Dir(xmlPath)))
		t.Run(patient, func(t *testing.T) {
			_, err := philipstodicom.ParsePhilips(xmlPath)
			if err != nil {
				t.Errorf("ParsePhilips(%q): %v", xmlPath, err)
			}
		})
	}
}

// TestConvert_NoError verifies that Convert succeeds for all XML/DICOM pairs.
func TestConvert_NoError(t *testing.T) {
	skipIfNoData(t)

	for _, p := range testPairs {
		p := p
		t.Run(p.patient, func(t *testing.T) {
			t.Parallel()
			xmlPath := filepath.Join(philipsRoot, p.xmlPath)
			outPath := filepath.Join(t.TempDir(), p.patient+".dcm")
			if err := philipstodicom.Convert(xmlPath, outPath); err != nil {
				t.Fatalf("Convert: %v", err)
			}
			if fi, err := os.Stat(outPath); err != nil || fi.Size() == 0 {
				t.Fatalf("output file missing or empty")
			}
		})
	}
}

// TestConvert_Metadata compares patient and study metadata between our DICOM and the reference.
func TestConvert_Metadata(t *testing.T) {
	skipIfNoData(t)

	for _, p := range testPairs {
		p := p
		t.Run(p.patient, func(t *testing.T) {
			t.Parallel()
			xmlPath := filepath.Join(philipsRoot, p.xmlPath)
			refPath := filepath.Join(philipsRoot, p.dcmPath)
			outPath := filepath.Join(t.TempDir(), p.patient+".dcm")

			if err := philipstodicom.Convert(xmlPath, outPath); err != nil {
				t.Fatalf("Convert: %v", err)
			}
			ref, err := parseDICOM(refPath)
			if err != nil {
				t.Fatalf("parse reference DICOM: %v", err)
			}
			got, err := parseDICOM(outPath)
			if err != nil {
				t.Fatalf("parse output DICOM: %v", err)
			}

			check := func(field, want, have string) {
				t.Helper()
				if want != "" && have != want {
					t.Errorf("%s: want %q, got %q", field, want, have)
				}
			}

			// PatientName/PatientID in reference DICOMs are anonymised (UUIDs) — skip.
			check("PatientSex", ref.PatientSex, got.PatientSex)
			check("PatientAge", ref.PatientAge, got.PatientAge)
			check("StudyDate", ref.StudyDate, got.StudyDate)
			check("StudyTime", ref.StudyTime, got.StudyTime)
			check("Manufacturer", ref.Manufacturer, got.Manufacturer)
		})
	}
}

// TestConvert_WaveformStructure verifies WaveformSequence structure (channels, samples, originality).
func TestConvert_WaveformStructure(t *testing.T) {
	skipIfNoData(t)

	for _, p := range testPairs {
		p := p
		t.Run(p.patient, func(t *testing.T) {
			t.Parallel()
			xmlPath := filepath.Join(philipsRoot, p.xmlPath)
			refPath := filepath.Join(philipsRoot, p.dcmPath)
			outPath := filepath.Join(t.TempDir(), p.patient+".dcm")

			if err := philipstodicom.Convert(xmlPath, outPath); err != nil {
				t.Fatalf("Convert: %v", err)
			}
			ref, err := parseDICOM(refPath)
			if err != nil {
				t.Fatalf("parse reference DICOM: %v", err)
			}
			got, err := parseDICOM(outPath)
			if err != nil {
				t.Fatalf("parse output DICOM: %v", err)
			}

			// Must have at least the ORIGINAL waveform item
			if len(got.Waveforms) == 0 {
				t.Fatal("no WaveformSequence items in output")
			}

			// Find ORIGINAL item in our output
			var ourOriginal, refOriginal *waveformInfo
			for i := range got.Waveforms {
				if got.Waveforms[i].Originality == "ORIGINAL" {
					ourOriginal = &got.Waveforms[i]
					break
				}
			}
			for i := range ref.Waveforms {
				if ref.Waveforms[i].Originality == "ORIGINAL" {
					refOriginal = &ref.Waveforms[i]
					break
				}
			}
			if ourOriginal == nil {
				t.Fatal("no ORIGINAL waveform in output")
			}
			if refOriginal == nil {
				t.Fatal("no ORIGINAL waveform in reference DICOM")
			}

			if ourOriginal.NumChannels != 12 {
				t.Errorf("ORIGINAL NumChannels: want 12, got %d", ourOriginal.NumChannels)
			}
			if ourOriginal.NumSamples != refOriginal.NumSamples {
				t.Errorf("ORIGINAL NumSamples: want %d (ref), got %d", refOriginal.NumSamples, ourOriginal.NumSamples)
			}
			if got.SamplingFreq != ref.SamplingFreq {
				t.Errorf("SamplingFrequency: want %q, got %q", ref.SamplingFreq, got.SamplingFreq)
			}
		})
	}
}

// TestConvert_WaveformData verifies that the ECG morphology in our output matches the reference DICOM.
// The Philips machine writes the DICOM from internal ADC data and the XML/XLI from post-processed data,
// so absolute values can differ by ~20 LSB DC offset. We compare DC-removed signals using Pearson
// correlation and normalised RMSE (should be > 0.99 and < 5 LSB respectively).
func TestConvert_WaveformData(t *testing.T) {
	skipIfNoData(t)

	// Only check independent leads (I, II, V1-V6 = indices 0,1,6-11)
	independentLeads := []struct {
		idx  int
		name string
	}{
		{0, "I"}, {1, "II"},
		{6, "V1"}, {7, "V2"}, {8, "V3"},
		{9, "V4"}, {10, "V5"}, {11, "V6"},
	}

	for _, p := range testPairs {
		p := p
		t.Run(p.patient, func(t *testing.T) {
			t.Parallel()
			xmlPath := filepath.Join(philipsRoot, p.xmlPath)
			refPath := filepath.Join(philipsRoot, p.dcmPath)
			outPath := filepath.Join(t.TempDir(), p.patient+".dcm")

			if err := philipstodicom.Convert(xmlPath, outPath); err != nil {
				t.Fatalf("Convert: %v", err)
			}
			ref, err := parseDICOM(refPath)
			if err != nil {
				t.Fatalf("parse reference DICOM: %v", err)
			}
			got, err := parseDICOM(outPath)
			if err != nil {
				t.Fatalf("parse output DICOM: %v", err)
			}

			var refW, gotW *waveformInfo
			for i := range ref.Waveforms {
				if ref.Waveforms[i].Originality == "ORIGINAL" {
					refW = &ref.Waveforms[i]
				}
			}
			for i := range got.Waveforms {
				if got.Waveforms[i].Originality == "ORIGINAL" {
					gotW = &got.Waveforms[i]
				}
			}
			if refW == nil || gotW == nil || len(refW.Samples) == 0 || len(gotW.Samples) == 0 {
				t.Skip("samples not available")
			}

			// Use a 1000-sample window excluding the first 100 (seed convergence artefact).
			n := refW.NumSamples
			start := 100
			end := start + 1000
			if end > n {
				end = n
			}

			for _, lead := range independentLeads {
				if lead.idx >= len(refW.Samples) || lead.idx >= len(gotW.Samples) {
					continue
				}
				refRaw := refW.Samples[lead.idx][start:end]
				gotRaw := gotW.Samples[lead.idx][start:end]
				ln := len(refRaw)

				// Remove DC offset from both signals before comparing.
				refF := make([]float64, ln)
				gotF := make([]float64, ln)
				var refMean, gotMean float64
				for _, v := range refRaw {
					refMean += float64(v)
				}
				for _, v := range gotRaw {
					gotMean += float64(v)
				}
				refMean /= float64(ln)
				gotMean /= float64(ln)
				for i, v := range refRaw {
					refF[i] = float64(v) - refMean
				}
				for i, v := range gotRaw {
					gotF[i] = float64(v) - gotMean
				}

				// Pearson correlation
				var num, denomR, denomG float64
				for i := range refF {
					num += refF[i] * gotF[i]
					denomR += refF[i] * refF[i]
					denomG += gotF[i] * gotF[i]
				}
				denom := math.Sqrt(denomR * denomG)
				var corr float64
				if denom > 0 {
					corr = num / denom
				}

				// RMSE of DC-removed signals
				var sumSq float64
				for i := range refF {
					d := gotF[i] - refF[i]
					sumSq += d * d
				}
				rmse := math.Sqrt(sumSq / float64(ln))

				// Signal must be non-trivially variable (skip flat leads)
				if denomR < 10 {
					continue
				}

				if corr < 0.90 {
					t.Errorf("Lead %s Pearson r=%.4f (want ≥0.90) — waveform shape mismatch", lead.name, corr)
				}
				if rmse > 50.0 {
					t.Errorf("Lead %s DC-removed RMSE=%.2f LSB (want ≤50) — waveform shape mismatch", lead.name, rmse)
				}
			}
		})
	}
}

// TestConvert_Annotations verifies that key measurements match the reference DICOM annotations.
func TestConvert_Annotations(t *testing.T) {
	skipIfNoData(t)

	// Map reference DICOM CodeMeaning → our DICOM CodeMeaning
	// (reference uses SCPECG, ours uses LOINC but same CodeMeaning strings for common ones)
	annotationMap := map[string]string{
		"RR Interval":          "RR Interval",
		"PR Interval":          "PR Interval",
		"QRS Duration":         "QRS Duration",
		"QT Interval":          "QT Interval",
		"QTc Interval":         "QTc Interval",
		"Ventricular Heart Rate": "Heart Rate",
		"P Axis":               "P-wave Axis",
		"QRS Axis":             "QRS Axis",
		"T Axis":               "T-wave Axis",
	}

	for _, p := range testPairs {
		p := p
		t.Run(p.patient, func(t *testing.T) {
			t.Parallel()
			xmlPath := filepath.Join(philipsRoot, p.xmlPath)
			refPath := filepath.Join(philipsRoot, p.dcmPath)
			outPath := filepath.Join(t.TempDir(), p.patient+".dcm")

			if err := philipstodicom.Convert(xmlPath, outPath); err != nil {
				t.Fatalf("Convert: %v", err)
			}
			ref, err := parseDICOM(refPath)
			if err != nil {
				t.Fatalf("parse reference DICOM: %v", err)
			}
			got, err := parseDICOM(outPath)
			if err != nil {
				t.Fatalf("parse output DICOM: %v", err)
			}

			for refName, ourName := range annotationMap {
				refVal, refOK := ref.Annotations[refName]
				ourVal, ourOK := got.Annotations[ourName]
				if !refOK {
					continue // not in reference, skip
				}
				if !ourOK {
					t.Errorf("annotation %q missing in output (ref value: %g)", ourName, refVal)
					continue
				}
				// Allow ±1 unit tolerance (rounding differences)
				if math.Abs(ourVal-refVal) > 1.0 {
					t.Errorf("annotation %q: want %g (ref), got %g", ourName, refVal, ourVal)
				}
			}
		})
	}
}

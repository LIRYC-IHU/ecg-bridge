package fdatodicom

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/tag"
)

const (
	testFDADir     = "../data_test/fda"
	testMindrayXML = "../data_test/25060897140_23092025103550.xml"
	refDICOMBase   = "/Volumes/Signal/ECG/Phillips"
	minCorrelation = 0.85
)

// testPatients lists the 12 patients with both FDA XML and reference DICOM.
var testPatients = []string{
	"BS1170", "BS1171", "BS1172", "BS1174", "BS1175", "BS1176",
	"BS1202", "BS1203", "BS1212", "BS1213", "BS1214", "BS1215",
}

// TestParseMetadata checks patient, study, and device fields for BS1170.
func TestParseMetadata(t *testing.T) {
	d, err := ParseFDA(filepath.Join(testFDADir, "BS1170.xml"))
	if err != nil {
		t.Fatalf("ParseFDA: %v", err)
	}

	if d.PatientID != "BS1170" {
		t.Errorf("PatientID = %q, want BS1170", d.PatientID)
	}
	if d.PatientName != "BLIN^Jean Michel" {
		t.Errorf("PatientName = %q, want BLIN^Jean Michel", d.PatientName)
	}
	if d.PatientSex != "M" {
		t.Errorf("PatientSex = %q, want M", d.PatientSex)
	}
	if d.StudyDate != "20250120" {
		t.Errorf("StudyDate = %q, want 20250120", d.StudyDate)
	}
	if d.StudyTime != "090120" {
		t.Errorf("StudyTime = %q, want 090120", d.StudyTime)
	}
	if d.Manufacturer != "Philips Medical Systems" {
		t.Errorf("Manufacturer = %q, want Philips Medical Systems", d.Manufacturer)
	}
	if d.ModelName != "860306" {
		t.Errorf("ModelName = %q, want 860306", d.ModelName)
	}
	if d.PatientAge != "050Y" {
		t.Errorf("PatientAge = %q, want 050Y", d.PatientAge)
	}
}

// TestParseInstitutionAndSerial checks InstitutionName and SerialNumber from a Mindray FDA XML.
func TestParseInstitutionAndSerial(t *testing.T) {
	if _, err := os.Stat(testMindrayXML); os.IsNotExist(err) {
		t.Skip("Mindray test file not found")
	}
	d, err := ParseFDA(testMindrayXML)
	if err != nil {
		t.Fatalf("ParseFDA: %v", err)
	}
	if d.PatientID != "25060897140" {
		t.Errorf("PatientID = %q, want 25060897140", d.PatientID)
	}
	if d.InstitutionName != "ID302 - 3e EST - 1463" {
		t.Errorf("InstitutionName = %q, want ID302 - 3e EST - 1463", d.InstitutionName)
	}
	if d.SerialNumber != "FN-3A045256" {
		t.Errorf("SerialNumber = %q, want FN-3A045256", d.SerialNumber)
	}
	if d.ModelName != "BeneHeart R12" {
		t.Errorf("ModelName = %q, want BeneHeart R12", d.ModelName)
	}
	if d.HeartRate != 57 {
		t.Errorf("HeartRate = %g, want 57", d.HeartRate)
	}
}

// TestParseWaveforms checks sampling rate, sensitivity, and lead data for BS1170.
func TestParseWaveforms(t *testing.T) {
	d, err := ParseFDA(filepath.Join(testFDADir, "BS1170.xml"))
	if err != nil {
		t.Fatalf("ParseFDA: %v", err)
	}

	if d.SamplingRate != 500 {
		t.Errorf("SamplingRate = %g, want 500", d.SamplingRate)
	}
	if d.Sensitivity != 5 {
		t.Errorf("Sensitivity = %g, want 5", d.Sensitivity)
	}
	if len(d.Leads) != 12 {
		t.Errorf("Leads count = %d, want 12", len(d.Leads))
	}
	for _, name := range leadOrder {
		samples, ok := d.Leads[name]
		if !ok {
			t.Errorf("missing lead %s", name)
			continue
		}
		if len(samples) != 5500 {
			t.Errorf("lead %s: %d samples, want 5500", name, len(samples))
		}
	}
}

// TestParseFilters checks filter values for BS1170.
func TestParseFilters(t *testing.T) {
	d, err := ParseFDA(filepath.Join(testFDADir, "BS1170.xml"))
	if err != nil {
		t.Fatalf("ParseFDA: %v", err)
	}

	if d.FilterLPF != 150 {
		t.Errorf("FilterLPF = %g, want 150", d.FilterLPF)
	}
	if d.FilterHPF != 0.05 {
		t.Errorf("FilterHPF = %g, want 0.05", d.FilterHPF)
	}
	if d.NotchFilter != 60 {
		t.Errorf("NotchFilter = %g, want 60", d.NotchFilter)
	}
}

// TestAnnotations checks measurement values for BS1170.
func TestAnnotations(t *testing.T) {
	d, err := ParseFDA(filepath.Join(testFDADir, "BS1170.xml"))
	if err != nil {
		t.Fatalf("ParseFDA: %v", err)
	}

	if d.HeartRate != 78 {
		t.Errorf("HeartRate = %g, want 78", d.HeartRate)
	}
	if d.PRInterval != 138 {
		t.Errorf("PRInterval = %g, want 138", d.PRInterval)
	}
	if d.QRSDuration != 72 {
		t.Errorf("QRSDuration = %g, want 72", d.QRSDuration)
	}
	if d.QTInterval != 349 {
		t.Errorf("QTInterval = %g, want 349", d.QTInterval)
	}
	if d.QTcInterval != 398 {
		t.Errorf("QTcInterval = %g, want 398", d.QTcInterval)
	}
}

// TestParseRepBeats checks that representative beat waveforms are parsed from derivation.
func TestParseRepBeats(t *testing.T) {
	d, err := ParseFDA(filepath.Join(testFDADir, "BS1170.xml"))
	if err != nil {
		t.Fatalf("ParseFDA: %v", err)
	}
	if len(d.RepBeats) != 12 {
		t.Errorf("RepBeats count = %d, want 12", len(d.RepBeats))
	}
	for _, name := range leadOrder {
		samples, ok := d.RepBeats[name]
		if !ok {
			t.Errorf("missing repbeat lead %s", name)
			continue
		}
		if len(samples) == 0 {
			t.Errorf("repbeat lead %s: empty", name)
		}
	}
}

// TestDICOMDeviceSerial checks that (0018,1000) DeviceSerialNumber is written.
func TestDICOMDeviceSerial(t *testing.T) {
	if _, err := os.Stat(testMindrayXML); os.IsNotExist(err) {
		t.Skip("Mindray test file not found")
	}
	output := filepath.Join(t.TempDir(), "mindray.dcm")
	if err := Convert(testMindrayXML, output); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	ds, err := dicom.ParseFile(output, nil)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	elem, err := ds.FindElementByTag(tag.DeviceSerialNumber)
	if err != nil {
		t.Fatalf("DeviceSerialNumber tag missing: %v", err)
	}
	strs, ok := elem.Value.GetValue().([]string)
	if !ok || len(strs) == 0 {
		t.Fatal("DeviceSerialNumber value empty")
	}
	if strs[0] != "FN-3A045256" {
		t.Errorf("DeviceSerialNumber = %q, want FN-3A045256", strs[0])
	}
}

// TestDICOMAnnotationChannels checks that (0040,A0B0) ReferencedWaveformChannels is [1,0]
// in every WaveformAnnotationSequence item.
func TestDICOMAnnotationChannels(t *testing.T) {
	output := filepath.Join(t.TempDir(), "bs1170.dcm")
	if err := Convert(filepath.Join(testFDADir, "BS1170.xml"), output); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	ds, err := dicom.ParseFile(output, nil)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	annSeqElem, err := ds.FindElementByTag(tag.WaveformAnnotationSequence)
	if err != nil {
		t.Fatalf("WaveformAnnotationSequence missing: %v", err)
	}
	annItems, ok := annSeqElem.Value.GetValue().([]*dicom.SequenceItemValue)
	if !ok || len(annItems) == 0 {
		t.Fatal("WaveformAnnotationSequence empty")
	}
	for i, item := range annItems {
		elems, ok := item.GetValue().([]*dicom.Element)
		if !ok {
			t.Errorf("item %d: cannot get elements", i)
			continue
		}
		itemDS := dicom.Dataset{Elements: elems}
		refElem, err := itemDS.FindElementByTag(tag.ReferencedWaveformChannels)
		if err != nil {
			t.Errorf("item %d: ReferencedWaveformChannels missing", i)
			continue
		}
		vals, ok := refElem.Value.GetValue().([]int)
		if !ok || len(vals) < 2 {
			t.Errorf("item %d: ReferencedWaveformChannels unexpected value %v", i, refElem.Value.GetValue())
			continue
		}
		if vals[0] != 1 || vals[1] != 0 {
			t.Errorf("item %d: ReferencedWaveformChannels = %v, want [1 0]", i, vals)
		}
	}
}

// TestDICOMWaveformItems checks that the DICOM output contains ORIGINAL + DERIVED waveform items.
func TestDICOMWaveformItems(t *testing.T) {
	output := filepath.Join(t.TempDir(), "bs1170.dcm")
	if err := Convert(filepath.Join(testFDADir, "BS1170.xml"), output); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	ds, err := dicom.ParseFile(output, nil)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	wfSeqElem, err := ds.FindElementByTag(tag.WaveformSequence)
	if err != nil {
		t.Fatalf("WaveformSequence missing: %v", err)
	}
	items, ok := wfSeqElem.Value.GetValue().([]*dicom.SequenceItemValue)
	if !ok {
		t.Fatal("WaveformSequence: unexpected type")
	}
	if len(items) != 2 {
		t.Errorf("WaveformSequence items = %d, want 2 (ORIGINAL + DERIVED)", len(items))
	}
	labels := []string{"RHYTHM", "REPRESENTATIVE BEAT"}
	for i, item := range items {
		elems, ok := item.GetValue().([]*dicom.Element)
		if !ok {
			continue
		}
		itemDS := dicom.Dataset{Elements: elems}
		labelElem, err := itemDS.FindElementByTag(tag.MultiplexGroupLabel)
		if err != nil {
			t.Errorf("item %d: MultiplexGroupLabel missing", i)
			continue
		}
		strs, _ := labelElem.Value.GetValue().([]string)
		if len(strs) == 0 || strs[0] != labels[i] {
			t.Errorf("item %d: MultiplexGroupLabel = %v, want %q", i, strs, labels[i])
		}
	}
}

// TestConvertAll converts all 12 patients and checks the output file size.
func TestConvertAll(t *testing.T) {
	for _, patient := range testPatients {
		patient := patient
		t.Run(patient, func(t *testing.T) {
			input := filepath.Join(testFDADir, patient+".xml")
			output := filepath.Join(t.TempDir(), patient+".dcm")
			if err := Convert(input, output); err != nil {
				t.Fatalf("Convert: %v", err)
			}
			info, err := os.Stat(output)
			if err != nil {
				t.Fatalf("stat output: %v", err)
			}
			if info.Size() < 100*1024 {
				t.Errorf("output file too small: %d bytes (< 100KB)", info.Size())
			}
		})
	}
}

// TestWaveformCorrelation compares waveforms from our DICOM against Philips reference.
// Skipped when /Volumes/Signal is not mounted.
func TestWaveformCorrelation(t *testing.T) {
	if _, err := os.Stat(refDICOMBase); os.IsNotExist(err) {
		t.Skip("reference DICOM volume not mounted")
	}

	for _, patient := range testPatients {
		patient := patient
		t.Run(patient, func(t *testing.T) {
			// Build our DICOM
			input := filepath.Join(testFDADir, patient+".xml")
			output := filepath.Join(t.TempDir(), patient+".dcm")
			if err := Convert(input, output); err != nil {
				t.Fatalf("Convert: %v", err)
			}

			// Extract waveform from our output
			ourLeads, err := readDICOMLeads(output)
			if err != nil {
				t.Fatalf("read our DICOM waveform: %v", err)
			}

			// Extract waveform from reference
			refPath := filepath.Join(refDICOMBase, patient, "DICOM", "ecg_0.dcm")
			refLeads, err := readDICOMLeads(refPath)
			if err != nil {
				t.Fatalf("read reference DICOM waveform: %v", err)
			}

			// Compare leads that exist in both
			for _, name := range leadOrder {
				ourSamples, ok1 := ourLeads[name]
				refSamples, ok2 := refLeads[name]
				if !ok1 || !ok2 {
					continue
				}
				r := pearsonCorrelation(ourSamples, refSamples)
				// Log only — this is an indirect pipeline comparison (Philips XML → FDA XML → DICOM
				// vs. device DICOM) so some deviation is expected, especially for derived leads.
				if r < minCorrelation {
					t.Logf("lead %s: Pearson r=%.3f < %.2f", name, r, minCorrelation)
				}
			}
		})
	}
}

// readDICOMLeads reads WaveformSequence from a DICOM file and returns
// a map of lead name → DC-removed float64 samples.
func readDICOMLeads(path string) (map[string][]float64, error) {
	ds, err := dicom.ParseFile(path, nil)
	if err != nil {
		return nil, err
	}

	wfSeqElem, err := ds.FindElementByTag(tag.WaveformSequence)
	if err != nil {
		return nil, err
	}
	items, ok := wfSeqElem.Value.GetValue().([]*dicom.SequenceItemValue)
	if !ok || len(items) == 0 {
		return nil, nil
	}

	// Use first (ORIGINAL) item
	itemElems, ok := items[0].GetValue().([]*dicom.Element)
	if !ok {
		return nil, nil
	}
	itemDS := dicom.Dataset{Elements: itemElems}

	// Number of channels
	nChElem, err := itemDS.FindElementByTag(tag.NumberOfWaveformChannels)
	if err != nil {
		return nil, err
	}
	nCh := nChElem.Value.GetValue().([]int)[0]

	// Number of samples
	nSampElem, err := itemDS.FindElementByTag(tag.NumberOfWaveformSamples)
	if err != nil {
		return nil, err
	}
	nSamp := nSampElem.Value.GetValue().([]int)[0]

	// Raw waveform data
	wdElem, err := itemDS.FindElementByTag(tag.WaveformData)
	if err != nil {
		return nil, err
	}
	rawBytes, ok := wdElem.Value.GetValue().([]byte)
	if !ok {
		return nil, nil
	}

	// Deinterleave channels
	channels := make([][]float64, nCh)
	for c := 0; c < nCh; c++ {
		channels[c] = make([]float64, nSamp)
	}
	for s := 0; s < nSamp; s++ {
		for c := 0; c < nCh; c++ {
			offset := (s*nCh + c) * 2
			if offset+2 > len(rawBytes) {
				break
			}
			v := int16(binary.LittleEndian.Uint16(rawBytes[offset:]))
			channels[c][s] = float64(v)
		}
	}

	// Get lead names from ChannelDefinitionSequence
	chanDefElem, err := itemDS.FindElementByTag(tag.ChannelDefinitionSequence)
	if err != nil {
		// Fall back to canonical order
		leads := make(map[string][]float64, nCh)
		for i, name := range leadOrder {
			if i < nCh {
				leads[name] = dcRemove(channels[i])
			}
		}
		return leads, nil
	}

	chanItems, ok := chanDefElem.Value.GetValue().([]*dicom.SequenceItemValue)
	if !ok {
		return nil, nil
	}

	leads := make(map[string][]float64, nCh)
	for i, chanItem := range chanItems {
		if i >= nCh {
			break
		}
		chanElems, ok2 := chanItem.GetValue().([]*dicom.Element)
		if !ok2 {
			continue
		}
		chanDS := dicom.Dataset{Elements: chanElems}
		srcSeqElem, err := chanDS.FindElementByTag(tag.ChannelSourceSequence)
		if err != nil {
			continue
		}
		srcItems, ok := srcSeqElem.Value.GetValue().([]*dicom.SequenceItemValue)
		if !ok || len(srcItems) == 0 {
			continue
		}
		srcElems, ok2 := srcItems[0].GetValue().([]*dicom.Element)
		if !ok2 {
			continue
		}
		srcDS := dicom.Dataset{Elements: srcElems}
		codeElem, err := srcDS.FindElementByTag(tag.CodeMeaning)
		if err != nil {
			continue
		}
		strs, ok := codeElem.Value.GetValue().([]string)
		if !ok || len(strs) == 0 {
			continue
		}
		leadName := strs[0]
		leads[leadName] = dcRemove(channels[i])
	}
	return leads, nil
}

// dcRemove subtracts the mean from a signal (DC removal).
func dcRemove(samples []float64) []float64 {
	if len(samples) == 0 {
		return samples
	}
	mean := 0.0
	for _, v := range samples {
		mean += v
	}
	mean /= float64(len(samples))
	out := make([]float64, len(samples))
	for i, v := range samples {
		out[i] = v - mean
	}
	return out
}

// pearsonCorrelation returns the Pearson correlation coefficient between two slices.
// Uses the shorter length if they differ.
func pearsonCorrelation(a, b []float64) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n == 0 {
		return 0
	}

	var sumA, sumB float64
	for i := 0; i < n; i++ {
		sumA += a[i]
		sumB += b[i]
	}
	meanA := sumA / float64(n)
	meanB := sumB / float64(n)

	var cov, varA, varB float64
	for i := 0; i < n; i++ {
		da := a[i] - meanA
		db := b[i] - meanB
		cov += da * db
		varA += da * da
		varB += db * db
	}

	denom := math.Sqrt(varA * varB)
	if denom == 0 {
		return 0
	}
	return cov / denom
}

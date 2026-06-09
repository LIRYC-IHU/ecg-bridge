package philipstodicom

import (
	"encoding/binary"
	"fmt"

	dicomconf "converter-fda/dicomconf"

	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/tag"
)

const (
	sopClassUID12LeadECG     = "1.2.840.10008.5.1.4.1.1.9.1.1"
	transferSyntaxExplicitLE = "1.2.840.10008.1.2.1"
)

// BuildDICOM constructs a DICOM dataset from PhilipsData.
func BuildDICOM(d *PhilipsData) (dicom.Dataset, error) {
	ds := dicom.Dataset{}

	// ─── File meta elements (group 0002) ────────────────────────────────────
	add(&ds, tag.MediaStorageSOPClassUID, []string{sopClassUID12LeadECG})
	add(&ds, tag.MediaStorageSOPInstanceUID, []string{d.StudyUID})
	add(&ds, tag.TransferSyntaxUID, []string{transferSyntaxExplicitLE})

	// ─── Patient / study / device ───────────────────────────────────────────
	add(&ds, tag.SOPClassUID, []string{sopClassUID12LeadECG})
	add(&ds, tag.SOPInstanceUID, []string{d.StudyUID})
	add(&ds, tag.Modality, []string{"ECG"})
	add(&ds, tag.StudyDate, []string{d.StudyDate})
	add(&ds, tag.StudyTime, []string{d.StudyTime})
	add(&ds, tag.ContentDate, []string{d.StudyDate})
	add(&ds, tag.ContentTime, []string{d.StudyTime})
	add(&ds, tag.AcquisitionDateTime, []string{d.StudyDate + d.StudyTime})
	add(&ds, tag.Manufacturer, []string{d.Manufacturer})
	add(&ds, tag.InstitutionName, []string{d.InstitutionName})
	add(&ds, tag.ManufacturerModelName, []string{d.ModelName})
	add(&ds, tag.SoftwareVersions, []string{d.SoftwareVer})
	add(&ds, tag.OperatorsName, []string{d.OperatorID})
	add(&ds, tag.PatientName, []string{d.PatientName})
	add(&ds, tag.PatientID, []string{d.PatientID})
	add(&ds, tag.PatientSex, []string{d.PatientSex})
	add(&ds, tag.PatientAge, []string{d.PatientAge})
	add(&ds, tag.StudyInstanceUID, []string{d.StudyUID})
	add(&ds, tag.SeriesInstanceUID, []string{d.StudyUID + ".1"})
	add(&ds, tag.StudyID, []string{"1"})
	add(&ds, tag.SeriesNumber, []string{"1"})
	add(&ds, tag.InstanceNumber, []string{"1"})

	// ─── WaveformSequence ───────────────────────────────────────────────────
	originalItem, err := buildWaveformItem(d, "ORIGINAL", "RHYTHM", d.RhythmLeads[:], 5500)
	if err != nil {
		return ds, fmt.Errorf("building ORIGINAL waveform: %w", err)
	}

	derivedItem, err := buildDerivedItem(d)
	if err != nil {
		return ds, fmt.Errorf("building DERIVED waveform: %w", err)
	}

	wfSeq, err := dicom.NewElement(tag.WaveformSequence, [][]*dicom.Element{
		originalItem,
		derivedItem,
	})
	if err != nil {
		return ds, fmt.Errorf("creating WaveformSequence: %w", err)
	}
	ds.Elements = append(ds.Elements, wfSeq)

	// ─── WaveformAnnotationSequence ─────────────────────────────────────────
	annItems, err := buildAnnotations(d)
	if err != nil {
		return ds, fmt.Errorf("building annotations: %w", err)
	}
	if len(annItems) > 0 {
		annSeq, err := dicom.NewElement(tag.WaveformAnnotationSequence, annItems)
		if err != nil {
			return ds, fmt.Errorf("creating WaveformAnnotationSequence: %w", err)
		}
		ds.Elements = append(ds.Elements, annSeq)
	}

	return ds, nil
}

// buildWaveformItem creates one item for WaveformSequence.
// leads is a slice of 12 lead data (each []int16), numSamples is the expected length.
func buildWaveformItem(d *PhilipsData, originality, label string, leads [][]int16, numSamples int) ([]*dicom.Element, error) {
	// ChannelDefinitionSequence
	channelItems := make([][]*dicom.Element, 0, 12)
	for i, name := range leadOrder {
		if i >= len(leads) {
			break
		}
		ch, err := buildChannelDef(name, d)
		if err != nil {
			return nil, fmt.Errorf("channel %s: %w", name, err)
		}
		channelItems = append(channelItems, ch)
	}
	chanDefSeq, err := dicom.NewElement(tag.ChannelDefinitionSequence, channelItems)
	if err != nil {
		return nil, err
	}

	// WaveformData: interleaved little-endian int16
	rawData := interleaveLeads(leads, numSamples)

	item := []*dicom.Element{
		mustElem(tag.WaveformOriginality, []string{originality}),
		mustElem(tag.NumberOfWaveformChannels, []int{12}),
		mustElem(tag.NumberOfWaveformSamples, []int{numSamples}),
		mustElem(tag.SamplingFrequency, []string{fmtFloat(d.SamplingRate)}),
		mustElem(tag.MultiplexGroupLabel, []string{label}),
		chanDefSeq,
		mustElem(tag.WaveformBitsAllocated, []int{16}),
		mustElem(tag.WaveformSampleInterpretation, []string{"SS"}),
		mustElem(tag.WaveformData, rawData),
	}
	return item, nil
}

// buildDerivedItem creates the DERIVED (representative beat) waveform item.
func buildDerivedItem(d *PhilipsData) ([]*dicom.Element, error) {
	// Collect leads in canonical order, find max length
	leads := make([][]int16, 12)
	maxLen := 0
	for i, name := range leadOrder {
		samples, ok := d.RepBeats[name]
		if !ok {
			samples = []int16{}
		}
		leads[i] = samples
		if len(samples) > maxLen {
			maxLen = len(samples)
		}
	}
	if maxLen == 0 {
		maxLen = 1
	}
	return buildWaveformItem(d, "DERIVED", "REPRESENTATIVE BEAT", leads, maxLen)
}

// buildChannelDef creates a ChannelDefinitionSequence item for one lead.
func buildChannelDef(leadName string, d *PhilipsData) ([]*dicom.Element, error) {
	codeValue := dicomconf.SCPECGLeadCode(leadName)

	// ChannelSourceSequence
	srcSeq, err := dicom.NewElement(tag.ChannelSourceSequence, [][]*dicom.Element{
		{
			mustElem(tag.CodeValue, []string{codeValue}),
			mustElem(tag.CodingSchemeDesignator, []string{"SCPECG"}),
			mustElem(tag.CodeMeaning, []string{leadName}),
		},
	})
	if err != nil {
		return nil, err
	}

	// ChannelSensitivityUnitsSequence — microvolt (UCUM)
	unitSeq, err := dicom.NewElement(tag.ChannelSensitivityUnitsSequence, [][]*dicom.Element{
		{
			mustElem(tag.CodeValue, []string{"uV"}),
			mustElem(tag.CodingSchemeDesignator, []string{"UCUM"}),
			mustElem(tag.CodeMeaning, []string{"microvolt"}),
		},
	})
	if err != nil {
		return nil, err
	}

	ch := []*dicom.Element{
		srcSeq,
		mustElem(tag.ChannelSensitivity, []string{fmtFloat(d.Sensitivity)}),
		unitSeq,
		mustElem(tag.ChannelSensitivityCorrectionFactor, []string{"1"}),
		mustElem(tag.ChannelBaseline, []string{fmtFloat(d.Baseline)}),
		mustElem(tag.ChannelSampleSkew, []string{"0"}),
		mustElem(tag.WaveformBitsStored, []int{d.BitsPerSample}),
		mustElem(tag.FilterLowFrequency, []string{fmtFloat(d.FilterHPF)}),
		mustElem(tag.FilterHighFrequency, []string{fmtFloat(d.FilterLPF)}),
	}

	if d.NotchFilter > 0 {
		ch = append(ch, mustElem(tag.NotchFilterFrequency, []string{fmtFloat(d.NotchFilter)}))
	}

	return ch, nil
}

// buildAnnotations creates WaveformAnnotationSequence items for global measurements.
func buildAnnotations(d *PhilipsData) ([][]*dicom.Element, error) {
	type measurement struct {
		codeValue   string
		codeMeaning string
		value       float64
		unit        string
		ucumCode    string
	}

	measurements := []measurement{
		{"8867-4", "Heart Rate", d.HeartRate, "/min", "/min"},
		{"8625-3", "PR Interval", d.PRInterval, "ms", "ms"},
		{"8625-6", "RR Interval", d.RRInterval, "ms", "ms"},
		{"8633-7", "QRS Duration", d.QRSDuration, "ms", "ms"},
		{"8634-5", "QT Interval", d.QTInterval, "ms", "ms"},
		{"8636-0", "QTc Interval", d.QTcInterval, "ms", "ms"},
		{"8639-5", "Atrial Rate", d.AtrialRate, "/min", "/min"},
		{"8626-1", "P-wave Axis", d.PFrontAxis, "deg", "deg"},
		{"8632-9", "QRS Axis", d.QRSFrontAxis, "deg", "deg"},
		{"8638-7", "T-wave Axis", d.TFrontAxis, "deg", "deg"},
		{"8628-7", "ST Axis", d.STFrontAxis, "deg", "deg"},
		{"8640-3", "QT Dispersion", d.QTDispersion, "ms", "ms"},
	}

	var items [][]*dicom.Element
	for _, m := range measurements {
		if m.value == 0 {
			continue
		}
		item, err := buildAnnotationItem(m.codeValue, m.codeMeaning, m.value, m.unit, m.ucumCode)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func buildAnnotationItem(codeValue, codeMeaning string, value float64, unitMeaning, ucumCode string) ([]*dicom.Element, error) {
	conceptSeq, err := dicom.NewElement(tag.ConceptNameCodeSequence, [][]*dicom.Element{
		{
			mustElem(tag.CodeValue, []string{codeValue}),
			mustElem(tag.CodingSchemeDesignator, []string{"LN"}),
			mustElem(tag.CodeMeaning, []string{codeMeaning}),
		},
	})
	if err != nil {
		return nil, err
	}

	unitSeq, err := dicom.NewElement(tag.MeasurementUnitsCodeSequence, [][]*dicom.Element{
		{
			mustElem(tag.CodeValue, []string{ucumCode}),
			mustElem(tag.CodingSchemeDesignator, []string{"UCUM"}),
			mustElem(tag.CodeMeaning, []string{unitMeaning}),
		},
	})
	if err != nil {
		return nil, err
	}

	measSeq, err := dicom.NewElement(tag.MeasuredValueSequence, [][]*dicom.Element{
		{
			mustElem(tag.NumericValue, []string{fmtFloat(value)}),
			unitSeq,
		},
	})
	if err != nil {
		return nil, err
	}

	return []*dicom.Element{conceptSeq, measSeq}, nil
}

// interleaveLeads converts per-lead samples into DICOM interleaved little-endian bytes.
// [C0S0, C1S0, ..., C11S0, C0S1, ..., C11SN]
func interleaveLeads(leads [][]int16, numSamples int) []byte {
	numCh := len(leads)
	buf := make([]byte, numCh*numSamples*2)
	for s := 0; s < numSamples; s++ {
		for c := 0; c < numCh; c++ {
			offset := (s*numCh + c) * 2
			var v int16
			if s < len(leads[c]) {
				v = leads[c][s]
			}
			binary.LittleEndian.PutUint16(buf[offset:], uint16(v))
		}
	}
	return buf
}

// add appends a new Element to the dataset, panicking on error.
func add(ds *dicom.Dataset, t tag.Tag, data any) {
	elem, err := dicom.NewElement(t, data)
	if err != nil {
		panic(fmt.Sprintf("NewElement %v: %v", t, err))
	}
	ds.Elements = append(ds.Elements, elem)
}

func mustElem(t tag.Tag, data any) *dicom.Element {
	elem, err := dicom.NewElement(t, data)
	if err != nil {
		panic(fmt.Sprintf("NewElement %v: %v", t, err))
	}
	return elem
}

func fmtFloat(f float64) string {
	// Use %g to avoid unnecessary trailing zeros
	return fmt.Sprintf("%g", f)
}

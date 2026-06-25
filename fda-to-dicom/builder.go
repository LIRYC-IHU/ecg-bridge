package fdatodicom

import (
	"encoding/binary"
	"fmt"

	dicomconf "github.com/LIRYC-IHU/ecg-bridge/dicomconf"

	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/tag"
)

// leadOrder is the canonical DICOM 12-lead order.
var leadOrder = []string{"I", "II", "III", "aVR", "aVL", "aVF", "V1", "V2", "V3", "V4", "V5", "V6"}

const (
	sopClassUID12LeadECG     = "1.2.840.10008.5.1.4.1.1.9.1.1"
	transferSyntaxExplicitLE = "1.2.840.10008.1.2.1"
)

// BuildDICOM constructs a DICOM dataset from FDAData.
func BuildDICOM(d *FDAData) (dicom.Dataset, error) {
	ds := dicom.Dataset{}

	// ── File meta (group 0002) ────────────────────────────────────────────────
	add(&ds, tag.MediaStorageSOPClassUID, []string{sopClassUID12LeadECG})
	add(&ds, tag.MediaStorageSOPInstanceUID, []string{d.StudyUID})
	add(&ds, tag.TransferSyntaxUID, []string{transferSyntaxExplicitLE})

	// ── Patient / study / device ──────────────────────────────────────────────
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
	add(&ds, tag.OperatorsName, []string{d.OperatorID})
	add(&ds, tag.ManufacturerModelName, []string{d.ModelName})
	add(&ds, tag.DeviceSerialNumber, []string{d.SerialNumber})
	add(&ds, tag.SoftwareVersions, []string{d.SoftwareVer})
	add(&ds, tag.PatientName, []string{d.PatientName})
	add(&ds, tag.PatientID, []string{d.PatientID})
	add(&ds, tag.PatientSex, []string{d.PatientSex})
	add(&ds, tag.PatientBirthDate, []string{d.PatientDOB})
	add(&ds, tag.PatientAge, []string{d.PatientAge})
	add(&ds, tag.StudyInstanceUID, []string{d.StudyUID})
	add(&ds, tag.SeriesInstanceUID, []string{d.StudyUID + ".1"})
	add(&ds, tag.StudyID, []string{"1"})
	add(&ds, tag.SeriesNumber, []string{"1"})
	add(&ds, tag.InstanceNumber, []string{"1"})

	// ── WaveformSequence ──────────────────────────────────────────────────────
	if len(d.Leads) > 0 {
		originalItem, err := buildWaveformItem(d, "ORIGINAL", "RHYTHM", d.Leads)
		if err != nil {
			return ds, fmt.Errorf("building ORIGINAL waveform: %w", err)
		}
		wfItems := [][]*dicom.Element{originalItem}

		if len(d.RepBeats) > 0 {
			derivedItem, err := buildWaveformItem(d, "DERIVED", "REPRESENTATIVE BEAT", d.RepBeats)
			if err != nil {
				return ds, fmt.Errorf("building DERIVED waveform: %w", err)
			}
			wfItems = append(wfItems, derivedItem)
		}

		wfSeq, err := dicom.NewElement(tag.WaveformSequence, wfItems)
		if err != nil {
			return ds, fmt.Errorf("creating WaveformSequence: %w", err)
		}
		ds.Elements = append(ds.Elements, wfSeq)
	}

	// ── WaveformAnnotationSequence ────────────────────────────────────────────
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

// buildWaveformItem creates one WaveformSequence item from a map of lead data.
func buildWaveformItem(d *FDAData, originality, label string, leads map[string][]int16) ([]*dicom.Element, error) {
	// Collect leads in canonical order, find max length
	orderedLeads := make([][]int16, 0, len(leadOrder))
	numSamples := 0
	for _, name := range leadOrder {
		samples := leads[name]
		orderedLeads = append(orderedLeads, samples)
		if len(samples) > numSamples {
			numSamples = len(samples)
		}
	}
	if numSamples == 0 {
		numSamples = 1
	}

	// ChannelDefinitionSequence
	channelItems := make([][]*dicom.Element, 0, len(leadOrder))
	for _, name := range leadOrder {
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

	// Normalize to 1 µV/LSB: multiply raw digits by sensitivity (+ baseline).
	// This ensures DICOM raw int16 values directly represent µV regardless of
	// the source file's original scale factor.
	sensitivity := d.Sensitivity
	if sensitivity == 0 {
		sensitivity = 1
	}
	scaledLeads := make([][]int16, len(orderedLeads))
	for i, lead := range orderedLeads {
		scaledLeads[i] = make([]int16, len(lead))
		for j, v := range lead {
			scaledLeads[i][j] = int16(float64(v)*sensitivity + d.Baseline)
		}
	}

	rawData := interleaveLeads(scaledLeads, numSamples)

	item := []*dicom.Element{
		mustElem(tag.WaveformOriginality, []string{originality}),
		mustElem(tag.NumberOfWaveformChannels, []int{len(leadOrder)}),
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

// buildChannelDef creates one ChannelDefinitionSequence item.
func buildChannelDef(leadName string, d *FDAData) ([]*dicom.Element, error) {
	codeValue := dicomconf.SCPECGLeadCode(leadName)

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
		mustElem(tag.ChannelSensitivity, []string{"1"}),
		unitSeq,
		mustElem(tag.ChannelSensitivityCorrectionFactor, []string{"1"}),
		mustElem(tag.ChannelBaseline, []string{"0"}),
		mustElem(tag.ChannelSampleSkew, []string{"0"}),
		mustElem(tag.WaveformBitsStored, []int{16}),
		// DICOM FilterLowFrequency = high-pass cutoff; FilterHighFrequency = low-pass cutoff
		mustElem(tag.FilterLowFrequency, []string{fmtFloat(d.FilterHPF)}),
		mustElem(tag.FilterHighFrequency, []string{fmtFloat(d.FilterLPF)}),
	}
	if d.NotchFilter > 0 {
		ch = append(ch, mustElem(tag.NotchFilterFrequency, []string{fmtFloat(d.NotchFilter)}))
	}
	return ch, nil
}

// interleaveLeads converts per-lead samples into DICOM interleaved little-endian bytes.
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

// buildAnnotations creates WaveformAnnotationSequence items for measurements.
func buildAnnotations(d *FDAData) ([][]*dicom.Element, error) {
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
		{"8633-7", "QRS Duration", d.QRSDuration, "ms", "ms"},
		{"8634-5", "QT Interval", d.QTInterval, "ms", "ms"},
		{"8636-0", "QTc Interval", d.QTcInterval, "ms", "ms"},
		{"8639-5", "Atrial Rate", d.AtrialRate, "/min", "/min"},
		{"8626-1", "P-wave Axis", d.PFrontAxis, "deg", "deg"},
		{"8632-9", "QRS Axis", d.QRSFrontAxis, "deg", "deg"},
		{"8638-7", "T-wave Axis", d.TFrontAxis, "deg", "deg"},
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

	// (0040,A0B0) ReferencedWaveformChannels: [waveform item index, channel index]
	// [1, 0] = waveform item 1 (ORIGINAL), all channels (global measurement)
	return []*dicom.Element{
		mustElem(tag.ReferencedWaveformChannels, []int{1, 0}),
		conceptSeq,
		measSeq,
	}, nil
}

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
	return fmt.Sprintf("%g", f)
}

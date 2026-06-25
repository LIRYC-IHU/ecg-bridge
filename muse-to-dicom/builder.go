package musetodicom

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math/big"

	dicomconf "github.com/LIRYC-IHU/ecg-bridge/dicomconf"
	musetofda "github.com/LIRYC-IHU/ecg-bridge/muse-to-fda"

	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/tag"
)

const (
	sopClassUID12LeadECG     = "1.2.840.10008.5.1.4.1.1.9.1.1"
	transferSyntaxExplicitLE = "1.2.840.10008.1.2.1"
	museManufacturer         = "GE Healthcare"
	museModelName            = "MUSE"
)

// leadOrder is the canonical 12-lead order, matching MuseData lead indices.
var leadOrder = [12]string{"I", "II", "III", "aVR", "aVL", "aVF", "V1", "V2", "V3", "V4", "V5", "V6"}


// BuildDICOM constructs a DICOM 12-lead ECG dataset from MuseData.
func BuildDICOM(d *musetofda.MuseData) (dicom.Dataset, error) {
	ds := dicom.Dataset{}

	// MUSE StudyUID is a UUID, not a valid DICOM UID — generate proper ones.
	sopUID := newDicomUID()
	studyUID := newDicomUID()
	seriesUID := newDicomUID()

	// ─── File meta elements (group 0002) ────────────────────────────────────
	add(&ds, tag.MediaStorageSOPClassUID, []string{sopClassUID12LeadECG})
	add(&ds, tag.MediaStorageSOPInstanceUID, []string{sopUID})
	add(&ds, tag.TransferSyntaxUID, []string{transferSyntaxExplicitLE})

	// ─── Patient / study / device ───────────────────────────────────────────
	add(&ds, tag.SOPClassUID, []string{sopClassUID12LeadECG})
	add(&ds, tag.SOPInstanceUID, []string{sopUID})
	add(&ds, tag.Modality, []string{"ECG"})
	add(&ds, tag.StudyDate, []string{d.StudyDate})
	add(&ds, tag.StudyTime, []string{d.StudyTime})
	add(&ds, tag.ContentDate, []string{d.StudyDate})
	add(&ds, tag.ContentTime, []string{d.StudyTime})
	add(&ds, tag.AcquisitionDateTime, []string{d.StudyDate + d.StudyTime})
	add(&ds, tag.Manufacturer, []string{museManufacturer})
	add(&ds, tag.ManufacturerModelName, []string{museModelName})
	add(&ds, tag.SoftwareVersions, []string{d.MuseVersion})
	add(&ds, tag.PatientName, []string{d.PatientName})
	add(&ds, tag.PatientID, []string{d.PatientID})
	add(&ds, tag.PatientSex, []string{d.PatientSex})
	add(&ds, tag.PatientAge, []string{d.PatientAge})
	add(&ds, tag.StudyInstanceUID, []string{studyUID})
	add(&ds, tag.SeriesInstanceUID, []string{seriesUID})
	add(&ds, tag.StudyID, []string{"1"})
	add(&ds, tag.SeriesNumber, []string{"1"})
	add(&ds, tag.InstanceNumber, []string{"1"})

	// ─── WaveformSequence (RHYTHM + MEDIAN BEAT) ────────────────────────────
	wfItems := make([][]*dicom.Element, 0, 2)

	if n := numSamples(d.RhythmLeads); n > 0 {
		item, err := buildWaveformItem(d, "ORIGINAL", "RHYTHM", d.RhythmLeads[:], n)
		if err != nil {
			return ds, fmt.Errorf("building RHYTHM waveform: %w", err)
		}
		wfItems = append(wfItems, item)
	}
	if n := numSamples(d.MedianLeads); n > 0 {
		item, err := buildWaveformItem(d, "DERIVED", "MEDIAN BEAT", d.MedianLeads[:], n)
		if err != nil {
			return ds, fmt.Errorf("building MEDIAN waveform: %w", err)
		}
		wfItems = append(wfItems, item)
	}

	if len(wfItems) > 0 {
		wfSeq, err := dicom.NewElement(tag.WaveformSequence, wfItems)
		if err != nil {
			return ds, fmt.Errorf("creating WaveformSequence: %w", err)
		}
		ds.Elements = append(ds.Elements, wfSeq)
	}

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
func buildWaveformItem(d *musetofda.MuseData, originality, label string, leads [][]int16, n int) ([]*dicom.Element, error) {
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

	rawData := interleaveLeads(leads, n)

	item := []*dicom.Element{
		mustElem(tag.WaveformOriginality, []string{originality}),
		mustElem(tag.NumberOfWaveformChannels, []int{12}),
		mustElem(tag.NumberOfWaveformSamples, []int{n}),
		mustElem(tag.SamplingFrequency, []string{fmtFloat(d.SamplingRate)}),
		mustElem(tag.MultiplexGroupLabel, []string{label}),
		chanDefSeq,
		mustElem(tag.WaveformBitsAllocated, []int{16}),
		mustElem(tag.WaveformSampleInterpretation, []string{"SS"}),
		mustElem(tag.WaveformData, rawData),
	}
	return item, nil
}

// buildChannelDef creates a ChannelDefinitionSequence item for one lead.
func buildChannelDef(leadName string, d *musetofda.MuseData) ([]*dicom.Element, error) {
	srcSeq, err := dicom.NewElement(tag.ChannelSourceSequence, [][]*dicom.Element{
		{
			mustElem(tag.CodeValue, []string{dicomconf.SCPECGLeadCode(leadName)}),
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
		mustElem(tag.ChannelSensitivity, []string{fmtFloat(d.Sensitivity)}),
		unitSeq,
		mustElem(tag.ChannelSensitivityCorrectionFactor, []string{"1"}),
		mustElem(tag.ChannelBaseline, []string{fmtFloat(d.Baseline)}),
		mustElem(tag.ChannelSampleSkew, []string{"0"}),
		mustElem(tag.WaveformBitsStored, []int{16}),
	}
	// MUSE filter settings (high-pass cutoff → low frequency limit, etc.)
	if d.FilterHPF > 0 {
		ch = append(ch, mustElem(tag.FilterLowFrequency, []string{fmtFloat(d.FilterHPF)}))
	}
	if d.FilterLPF > 0 {
		ch = append(ch, mustElem(tag.FilterHighFrequency, []string{fmtFloat(d.FilterLPF)}))
	}
	if d.NotchFilter > 0 {
		ch = append(ch, mustElem(tag.NotchFilterFrequency, []string{fmtFloat(d.NotchFilter)}))
	}
	return ch, nil
}

// buildAnnotations creates WaveformAnnotationSequence items for global measurements.
func buildAnnotations(d *musetofda.MuseData) ([][]*dicom.Element, error) {
	type measurement struct {
		codeValue   string
		codeMeaning string
		value       float64
		unit        string
	}

	measurements := []measurement{
		{"8867-4", "Heart Rate", d.HeartRate, "/min"},
		{"8639-5", "Atrial Rate", d.AtrialRate, "/min"},
		{"8625-3", "PR Interval", d.PRInterval, "ms"},
		{"8633-7", "QRS Duration", d.QRSDuration, "ms"},
		{"8634-5", "QT Interval", d.QTInterval, "ms"},
		{"8636-0", "QTc Interval", d.QTcInterval, "ms"},
		{"8626-1", "P-wave Axis", d.PFrontAxis, "deg"},
		{"8632-9", "QRS Axis", d.QRSFrontAxis, "deg"},
		{"8638-7", "T-wave Axis", d.TFrontAxis, "deg"},
	}

	var items [][]*dicom.Element
	for _, m := range measurements {
		if m.value == 0 {
			continue
		}
		item, err := buildAnnotationItem(m.codeValue, m.codeMeaning, m.value, m.unit)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	// Diagnosis statements as text annotations.
	for _, stmt := range d.DiagnosisStatements {
		item, err := buildTextAnnotationItem(stmt)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func buildAnnotationItem(codeValue, codeMeaning string, value float64, unit string) ([]*dicom.Element, error) {
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
			mustElem(tag.CodeValue, []string{unit}),
			mustElem(tag.CodingSchemeDesignator, []string{"UCUM"}),
			mustElem(tag.CodeMeaning, []string{unit}),
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

func buildTextAnnotationItem(text string) ([]*dicom.Element, error) {
	return []*dicom.Element{
		mustElem(tag.UnformattedTextValue, []string{text}),
	}, nil
}

// interleaveLeads converts per-lead samples into DICOM interleaved
// little-endian int16 bytes: [C0S0, C1S0, ..., C11S0, C0S1, ...].
func interleaveLeads(leads [][]int16, n int) []byte {
	numCh := len(leads)
	buf := make([]byte, numCh*n*2)
	for s := 0; s < n; s++ {
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

func numSamples(leads [12][]int16) int {
	max := 0
	for _, s := range leads {
		if len(s) > max {
			max = len(s)
		}
	}
	return max
}

// newDicomUID returns a valid DICOM UID derived from a random 128-bit
// value using the "2.25.<integer>" scheme (DICOM PS3.5 §B.2).
func newDicomUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	n := new(big.Int).SetBytes(b)
	return "2.25." + n.String()
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

package mfertodicom

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math/big"

	dicomconf "github.com/LIRYC-IHU/ecg-bridge/dicomconf"
	mfertofda "github.com/LIRYC-IHU/ecg-bridge/mfer-to-fda"

	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/tag"
)

const (
	sopClassUID12LeadECG     = "1.2.840.10008.5.1.4.1.1.9.1.1"
	transferSyntaxExplicitLE = "1.2.840.10008.1.2.1"
)

// leadOrder is the canonical 12-lead order, matching MferData lead indices.
var leadOrder = [12]string{"I", "II", "III", "aVR", "aVL", "aVF", "V1", "V2", "V3", "V4", "V5", "V6"}

// BuildDICOM constructs a DICOM 12-lead ECG dataset from MferData.
func BuildDICOM(d *mfertofda.MferData) (dicom.Dataset, error) {
	ds := dicom.Dataset{}

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
	add(&ds, tag.Manufacturer, []string{d.Manufacturer})
	add(&ds, tag.ManufacturerModelName, []string{d.ModelName})
	add(&ds, tag.SoftwareVersions, []string{d.SoftwareVer})
	add(&ds, tag.PatientName, []string{d.PatientName})
	add(&ds, tag.PatientID, []string{d.PatientID})
	add(&ds, tag.PatientSex, []string{d.PatientSex})
	add(&ds, tag.PatientBirthDate, []string{d.BirthDate})
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

	return ds, nil
}

// buildWaveformItem creates one item for WaveformSequence.
func buildWaveformItem(d *mfertofda.MferData, originality, label string, leads [][]int16, n int) ([]*dicom.Element, error) {
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
		mustElem(tag.SamplingFrequency, []string{fmtFloat(d.SampleRate)}),
		mustElem(tag.MultiplexGroupLabel, []string{label}),
		chanDefSeq,
		mustElem(tag.WaveformBitsAllocated, []int{16}),
		mustElem(tag.WaveformSampleInterpretation, []string{"SS"}),
		mustElem(tag.WaveformData, rawData),
	}
	return item, nil
}

// buildChannelDef creates a ChannelDefinitionSequence item for one lead.
func buildChannelDef(leadName string, d *mfertofda.MferData) ([]*dicom.Element, error) {
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
		mustElem(tag.ChannelSensitivity, []string{fmtFloat(d.Scale)}),
		unitSeq,
		mustElem(tag.ChannelSensitivityCorrectionFactor, []string{"1"}),
		mustElem(tag.ChannelBaseline, []string{"0"}),
		mustElem(tag.ChannelSampleSkew, []string{"0"}),
		mustElem(tag.WaveformBitsStored, []int{16}),
	}
	if d.FilterHPF > 0 {
		ch = append(ch, mustElem(tag.FilterLowFrequency, []string{fmtFloat(d.FilterHPF)}))
	}
	if d.NotchFilter > 0 {
		ch = append(ch, mustElem(tag.NotchFilterFrequency, []string{fmtFloat(d.NotchFilter)}))
	}
	return ch, nil
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

// newDicomUID returns a valid DICOM UID using the "2.25.<integer>" scheme.
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

package dicomtofda

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/tag"
)

// DicomData holds all extracted DICOM data needed for FDA XML output.
type DicomData struct {
	Patient         PatientMetadata
	StudyDate       string
	StudyTime       string
	StudyInstanceUID string
	Waveforms       []WaveformGroup
	Annotations     []WaveformAnnotation
}

// WaveformAnnotation holds one item from the DICOM WaveformAnnotationSequence.
// It can be either a numeric measurement or a text annotation.
type WaveformAnnotation struct {
	// ConceptName is the CodeMeaning of ConceptNameCodeSequence (e.g. "RR Interval")
	ConceptName string
	// Code and CodeSystem from ConceptNameCodeSequence
	Code       string
	CodeSystem string
	// NumericValue and unit for numeric measurements
	NumericValue string
	Unit         string
	// TextValue for text annotations / interpretations
	TextValue string
	// ReferencedChannels lists the waveform/channel indices (pairs: multiplex group, channel)
	ReferencedChannels []int
}

// WaveformGroup represents one multiplex group from the DICOM WaveformSequence.
type WaveformGroup struct {
	SamplingFrequency    float64
	NumberOfChannels     int
	NumberOfSamples      int
	BitsAllocated        int
	SampleInterpretation string // "SS" signed, "US" unsigned
	Originality          string // "ORIGINAL" or "DERIVED"
	Channels             []ChannelDef
	RawData              []byte
}

// ChannelDef holds per-channel metadata from ChannelDefinitionSequence.
type ChannelDef struct {
	Label                string
	SourceName           string
	Sensitivity          float64
	SensitivityUnit      string
	Baseline             float64
	FilterLowFrequency   float64
	FilterHighFrequency  float64
	NotchFilterFrequency float64
}

// Samples decodes interleaved raw bytes into per-channel int16 slices.
// DICOM layout: [C1S1, C2S1, ..., CnS1, C1S2, C2S2, ..., CnSm]
func (g *WaveformGroup) Samples() [][]int16 {
	channels := make([][]int16, g.NumberOfChannels)
	for i := range channels {
		channels[i] = make([]int16, 0, g.NumberOfSamples)
	}
	bytesPerSample := g.BitsAllocated / 8
	if bytesPerSample == 0 {
		bytesPerSample = 2
	}
	for s := 0; s < g.NumberOfSamples; s++ {
		for c := 0; c < g.NumberOfChannels; c++ {
			offset := (s*g.NumberOfChannels + c) * bytesPerSample
			if offset+bytesPerSample > len(g.RawData) {
				break
			}
			raw := binary.LittleEndian.Uint16(g.RawData[offset : offset+2])
			channels[c] = append(channels[c], int16(raw))
		}
	}
	return channels
}

// ParseDicom reads a DICOM file and returns extracted data.
func ParseDicom(path string) (*DicomData, error) {
	dataset, err := dicom.ParseFile(path, nil)
	if err != nil {
		return nil, fmt.Errorf("parsing DICOM file: %w", err)
	}

	data := &DicomData{}
	data.Patient.PatientName = getString(dataset, tag.PatientName)
	data.Patient.PatientID = getString(dataset, tag.PatientID)
	data.Patient.PatientBirthDate = getString(dataset, tag.PatientBirthDate)
	data.Patient.PatientSex = getString(dataset, tag.PatientSex)
	data.Patient.PatientAge = getString(dataset, tag.PatientAge)
	data.Patient.DeviceModel = getString(dataset, tag.ManufacturerModelName)
	data.Patient.DeviceSerial = getString(dataset, tag.DeviceSerialNumber)
	data.Patient.SoftwareVersion = getString(dataset, tag.SoftwareVersions)
	data.Patient.Manufacturer = getString(dataset, tag.Manufacturer)
	data.Patient.OperatorsName = getString(dataset, tag.OperatorsName)
	data.Patient.InstitutionName = getString(dataset, tag.InstitutionName)
	data.Patient.InvestigatorName = getString(dataset, tag.ReferringPhysicianName)
	data.StudyDate = getString(dataset, tag.StudyDate)
	data.StudyTime = getString(dataset, tag.StudyTime)
	data.StudyInstanceUID = getString(dataset, tag.StudyInstanceUID)

	// Philips fallback: date/time absent → extract from StudyInstanceUID
	// Pattern: 1.2.276.0.7230010.3.X.X.MACHINE.COUNTER.UNIXTIMESTAMP.SERIAL
	if data.StudyDate == "" && data.StudyInstanceUID != "" {
		data.StudyDate, data.StudyTime = dateFromPhilipsUID(data.StudyInstanceUID)
	}

	// Extract observations from WaveformAnnotationSequence (dataset level)
	if annElem, err := dataset.FindElementByTag(tag.WaveformAnnotationSequence); err == nil {
		if annItems, ok := annElem.Value.GetValue().([]*dicom.SequenceItemValue); ok {
			for _, item := range annItems {
				elements := item.GetValue().([]*dicom.Element)
				data.Annotations = append(data.Annotations, parseAnnotation(elements))
			}
		}
	}

	wfSeqElem, err := dataset.FindElementByTag(tag.WaveformSequence)
	if err != nil {
		return data, nil // no waveform, not an error
	}

	seqItems, ok := wfSeqElem.Value.GetValue().([]*dicom.SequenceItemValue)
	if !ok {
		return data, nil
	}

	for _, item := range seqItems {
		elements := item.GetValue().([]*dicom.Element)
		g := parseWaveformGroup(elements)
		data.Waveforms = append(data.Waveforms, g)
	}

	return data, nil
}

func parseWaveformGroup(elements []*dicom.Element) WaveformGroup {
	var g WaveformGroup

	if e := findElement(elements, tag.SamplingFrequency); e != nil {
		g.SamplingFrequency = parseFloat(elemString(e))
	}
	if e := findElement(elements, tag.NumberOfWaveformChannels); e != nil {
		g.NumberOfChannels = elemInt(e)
	}
	if e := findElement(elements, tag.NumberOfWaveformSamples); e != nil {
		g.NumberOfSamples = elemInt(e)
	}
	if e := findElement(elements, tag.WaveformBitsAllocated); e != nil {
		g.BitsAllocated = elemInt(e)
	}
	if e := findElement(elements, tag.WaveformSampleInterpretation); e != nil {
		g.SampleInterpretation = elemString(e)
	}
	if e := findElement(elements, tag.WaveformOriginality); e != nil {
		g.Originality = elemString(e)
	}
	if e := findElement(elements, tag.WaveformData); e != nil {
		if raw, ok := e.Value.GetValue().([]byte); ok {
			g.RawData = raw
		}
	}

	if e := findElement(elements, tag.ChannelDefinitionSequence); e != nil {
		if chItems, ok := e.Value.GetValue().([]*dicom.SequenceItemValue); ok {
			for _, chItem := range chItems {
				chElements := chItem.GetValue().([]*dicom.Element)
				g.Channels = append(g.Channels, parseChannelDef(chElements))
			}
		}
	}

	return g
}

func parseChannelDef(elements []*dicom.Element) ChannelDef {
	var ch ChannelDef

	if e := findElement(elements, tag.ChannelLabel); e != nil {
		ch.Label = elemString(e)
	}
	if e := findElement(elements, tag.ChannelSensitivity); e != nil {
		ch.Sensitivity = parseFloat(elemString(e))
	}
	if e := findElement(elements, tag.ChannelBaseline); e != nil {
		ch.Baseline = parseFloat(elemString(e))
	}
	if e := findElement(elements, tag.FilterLowFrequency); e != nil {
		ch.FilterLowFrequency = parseFloat(elemString(e))
	}
	if e := findElement(elements, tag.FilterHighFrequency); e != nil {
		ch.FilterHighFrequency = parseFloat(elemString(e))
	}
	if e := findElement(elements, tag.NotchFilterFrequency); e != nil {
		ch.NotchFilterFrequency = parseFloat(elemString(e))
	}

	// Units from ChannelSensitivityUnitsSequence > CodeMeaning
	if e := findElement(elements, tag.ChannelSensitivityUnitsSequence); e != nil {
		if units, ok := e.Value.GetValue().([]*dicom.SequenceItemValue); ok && len(units) > 0 {
			unitElems := units[0].GetValue().([]*dicom.Element)
			if cv := findElement(unitElems, tag.CodeMeaning); cv != nil {
				ch.SensitivityUnit = elemString(cv)
			}
		}
	}

	// Lead name from ChannelSourceSequence > CodeMeaning
	if e := findElement(elements, tag.ChannelSourceSequence); e != nil {
		if src, ok := e.Value.GetValue().([]*dicom.SequenceItemValue); ok && len(src) > 0 {
			srcElems := src[0].GetValue().([]*dicom.Element)
			if cv := findElement(srcElems, tag.CodeMeaning); cv != nil {
				ch.SourceName = elemString(cv)
			}
		}
	}

	return ch
}

func parseAnnotation(elements []*dicom.Element) WaveformAnnotation {
	var a WaveformAnnotation

	// ConceptNameCodeSequence → concept name + code
	if e := findElement(elements, tag.ConceptNameCodeSequence); e != nil {
		if items, ok := e.Value.GetValue().([]*dicom.SequenceItemValue); ok && len(items) > 0 {
			elems := items[0].GetValue().([]*dicom.Element)
			if cv := findElement(elems, tag.CodeMeaning); cv != nil {
				a.ConceptName = elemString(cv)
			}
			if cv := findElement(elems, tag.CodeValue); cv != nil {
				a.Code = elemString(cv)
			}
			if cv := findElement(elems, tag.CodingSchemeDesignator); cv != nil {
				a.CodeSystem = elemString(cv)
			}
		}
	}

	// TextValue → interpretation text
	if e := findElement(elements, tag.TextValue); e != nil {
		a.TextValue = elemString(e)
	}

	// MeasuredValueSequence → numeric value + unit (standard layout: Mindray, GE…)
	if e := findElement(elements, tag.MeasuredValueSequence); e != nil {
		if items, ok := e.Value.GetValue().([]*dicom.SequenceItemValue); ok && len(items) > 0 {
			mvElems := items[0].GetValue().([]*dicom.Element)
			if nv := findElement(mvElems, tag.NumericValue); nv != nil {
				a.NumericValue = elemString(nv)
			}
			if ue := findElement(mvElems, tag.MeasurementUnitsCodeSequence); ue != nil {
				if unitItems, ok := ue.Value.GetValue().([]*dicom.SequenceItemValue); ok && len(unitItems) > 0 {
					unitElems := unitItems[0].GetValue().([]*dicom.Element)
					if cv := findElement(unitElems, tag.CodeMeaning); cv != nil {
						a.Unit = elemString(cv)
					}
				}
			}
		}
	}

	// Philips layout: NumericValue and MeasurementUnitsCodeSequence directly on the item
	if a.NumericValue == "" {
		if nv := findElement(elements, tag.NumericValue); nv != nil {
			a.NumericValue = elemString(nv)
		}
	}
	if a.Unit == "" {
		if ue := findElement(elements, tag.MeasurementUnitsCodeSequence); ue != nil {
			if unitItems, ok := ue.Value.GetValue().([]*dicom.SequenceItemValue); ok && len(unitItems) > 0 {
				unitElems := unitItems[0].GetValue().([]*dicom.Element)
				if cv := findElement(unitElems, tag.CodeMeaning); cv != nil {
					a.Unit = elemString(cv)
				}
			}
		}
	}

	// ReferencedWaveformChannels → list of uint16 pairs (multiplex group idx, channel idx)
	if e := findElement(elements, tag.ReferencedWaveformChannels); e != nil {
		if vals, ok := e.Value.GetValue().([]int); ok {
			a.ReferencedChannels = vals
		}
	}

	return a
}

// helpers

func findElement(elements []*dicom.Element, t tag.Tag) *dicom.Element {
	for _, e := range elements {
		if e.Tag == t {
			return e
		}
	}
	return nil
}

func getString(ds dicom.Dataset, t tag.Tag) string {
	elem, err := ds.FindElementByTag(t)
	if err != nil {
		return ""
	}
	return elemString(elem)
}

func elemString(elem *dicom.Element) string {
	vals, ok := elem.Value.GetValue().([]string)
	if !ok || len(vals) == 0 {
		return ""
	}
	return strings.TrimSpace(vals[0])
}

func elemInt(elem *dicom.Element) int {
	switch v := elem.Value.GetValue().(type) {
	case []int:
		if len(v) > 0 {
			return v[0]
		}
	case []string:
		if len(v) > 0 {
			n, _ := strconv.Atoi(strings.TrimSpace(v[0]))
			return n
		}
	}
	return 0
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

// dateFromPhilipsUID extracts date/time from a Philips StudyInstanceUID.
// Philips embeds a Unix timestamp as the penultimate numeric component:
// 1.2.276.0.7230010.3.X.X.MACHINE.COUNTER.UNIXTIMESTAMP.SERIAL
func dateFromPhilipsUID(uid string) (date, timeStr string) {
	parts := strings.Split(uid, ".")
	// Try each component from the right, skip last (serial), find a plausible Unix ts
	for i := len(parts) - 2; i >= 0; i-- {
		ts, err := strconv.ParseInt(parts[i], 10, 64)
		if err != nil {
			continue
		}
		// Unix timestamps between 2000-01-01 and 2100-01-01
		if ts >= 946684800 && ts <= 4102444800 {
			t := time.Unix(ts, 0).UTC()
			return t.Format("20060102"), t.Format("150405")
		}
	}
	return "", ""
}

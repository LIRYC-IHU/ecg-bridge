package mindraytodicom

import (
	"encoding/binary"
	"fmt"

	dicomconf "github.com/LIRYC-IHU/ecg-bridge/dicomconf"
	mindraytofda "github.com/LIRYC-IHU/ecg-bridge/mindray-to-fda"

	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/tag"
)

const (
	sopClassUID12LeadECG     = "1.2.840.10008.5.1.4.1.1.9.1.1"
	transferSyntaxExplicitLE = "1.2.840.10008.1.2.1"
)

var leadOrder = [12]string{"I", "II", "III", "aVR", "aVL", "aVF", "V1", "V2", "V3", "V4", "V5", "V6"}

func BuildDICOM(md *mindraytofda.MindrayData) (dicom.Dataset, error) {
	ds := dicom.Dataset{}

	studyUID := buildStudyUID(md)
	studyDate := ""
	studyTime := ""
	if !md.Patient.StartTime.IsZero() {
		studyDate = md.Patient.StartTime.Format("20060102")
		studyTime = md.Patient.StartTime.Format("150405")
	}

	// File meta
	add(&ds, tag.MediaStorageSOPClassUID, []string{sopClassUID12LeadECG})
	add(&ds, tag.MediaStorageSOPInstanceUID, []string{studyUID})
	add(&ds, tag.TransferSyntaxUID, []string{transferSyntaxExplicitLE})

	// Patient / Study
	add(&ds, tag.SOPClassUID, []string{sopClassUID12LeadECG})
	add(&ds, tag.SOPInstanceUID, []string{studyUID})
	add(&ds, tag.Modality, []string{"ECG"})
	add(&ds, tag.StudyDate, []string{studyDate})
	add(&ds, tag.StudyTime, []string{studyTime})
	add(&ds, tag.ContentDate, []string{studyDate})
	add(&ds, tag.ContentTime, []string{studyTime})
	add(&ds, tag.AcquisitionDateTime, []string{studyDate + studyTime})
	add(&ds, tag.Manufacturer, []string{md.Device.Manufacturer})
	add(&ds, tag.InstitutionName, []string{md.Patient.Location})
	add(&ds, tag.ManufacturerModelName, []string{md.Device.ModelName})
	add(&ds, tag.SoftwareVersions, []string{md.Device.SoftwareName})
	add(&ds, tag.DeviceSerialNumber, []string{md.Device.SerialNumber})
	add(&ds, tag.PatientName, []string{md.Patient.Name})
	add(&ds, tag.PatientID, []string{md.Patient.PatientID})
	add(&ds, tag.PatientSex, []string{dicomGender(md.Patient.Gender)})
	add(&ds, tag.StudyInstanceUID, []string{studyUID})
	add(&ds, tag.SeriesInstanceUID, []string{studyUID + ".1"})
	add(&ds, tag.StudyID, []string{"1"})
	add(&ds, tag.SeriesNumber, []string{"1"})
	add(&ds, tag.InstanceNumber, []string{"1"})

	// Waveform
	numSamples := findNumSamples(md.Leads)
	wfItem, err := buildWaveformItem(md, numSamples)
	if err != nil {
		return ds, fmt.Errorf("building waveform: %w", err)
	}

	wfSeq, err := dicom.NewElement(tag.WaveformSequence, [][]*dicom.Element{wfItem})
	if err != nil {
		return ds, fmt.Errorf("creating WaveformSequence: %w", err)
	}
	ds.Elements = append(ds.Elements, wfSeq)

	// Annotations
	annItems, err := buildAnnotations(md)
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

func buildWaveformItem(md *mindraytofda.MindrayData, numSamples int) ([]*dicom.Element, error) {
	channelItems := make([][]*dicom.Element, 0, 12)
	for _, name := range leadOrder {
		ch, err := buildChannelDef(name, md)
		if err != nil {
			return nil, fmt.Errorf("channel %s: %w", name, err)
		}
		channelItems = append(channelItems, ch)
	}
	chanDefSeq, err := dicom.NewElement(tag.ChannelDefinitionSequence, channelItems)
	if err != nil {
		return nil, err
	}

	rawData := interleaveLeads(md.Leads, numSamples)

	item := []*dicom.Element{
		mustElem(tag.WaveformOriginality, []string{"ORIGINAL"}),
		mustElem(tag.NumberOfWaveformChannels, []int{12}),
		mustElem(tag.NumberOfWaveformSamples, []int{numSamples}),
		mustElem(tag.SamplingFrequency, []string{fmt.Sprintf("%d", md.Record.SampleRate)}),
		mustElem(tag.MultiplexGroupLabel, []string{"RHYTHM"}),
		chanDefSeq,
		mustElem(tag.WaveformBitsAllocated, []int{16}),
		mustElem(tag.WaveformSampleInterpretation, []string{"SS"}),
		mustElem(tag.WaveformData, rawData),
	}
	return item, nil
}

func buildChannelDef(leadName string, md *mindraytofda.MindrayData) ([]*dicom.Element, error) {
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

	sensitivity := fmt.Sprintf("%g", md.Record.Scale)

	ch := []*dicom.Element{
		srcSeq,
		mustElem(tag.ChannelSensitivity, []string{sensitivity}),
		unitSeq,
		mustElem(tag.ChannelSensitivityCorrectionFactor, []string{"1"}),
		mustElem(tag.ChannelBaseline, []string{"0"}),
		mustElem(tag.ChannelSampleSkew, []string{"0"}),
		mustElem(tag.WaveformBitsStored, []int{16}),
	}
	return ch, nil
}

func buildAnnotations(md *mindraytofda.MindrayData) ([][]*dicom.Element, error) {
	type measurement struct {
		codeValue   string
		codeMeaning string
		value       float64
		unit        string
	}

	m := md.Measurement
	measurements := []measurement{
		{"8867-4", "Heart Rate", float64(m.HeartRate), "/min"},
		{"8625-3", "PR Interval", float64(m.PRInterval), "ms"},
		{"8633-7", "QRS Duration", float64(m.QRSDuration), "ms"},
		{"8634-5", "QT Interval", float64(m.QTInterval), "ms"},
		{"8636-0", "QTc Interval", float64(m.QTcInterval), "ms"},
	}
	if m.HasPAxis {
		measurements = append(measurements, measurement{"8626-1", "P-wave Axis", float64(m.PAxis), "deg"})
	}
	if m.HasQRSAxis {
		measurements = append(measurements, measurement{"8632-9", "QRS Axis", float64(m.QRSAxis), "deg"})
	}
	if m.HasTAxis {
		measurements = append(measurements, measurement{"8638-7", "T-wave Axis", float64(m.TAxis), "deg"})
	}

	var items [][]*dicom.Element
	for _, ms := range measurements {
		if ms.value == 0 {
			continue
		}
		item, err := buildAnnotationItem(ms.codeValue, ms.codeMeaning, ms.value, ms.unit)
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
			mustElem(tag.NumericValue, []string{fmt.Sprintf("%g", value)}),
			unitSeq,
		},
	})
	if err != nil {
		return nil, err
	}

	return []*dicom.Element{conceptSeq, measSeq}, nil
}

func interleaveLeads(leads map[string][]int, numSamples int) []byte {
	numCh := 12
	buf := make([]byte, numCh*numSamples*2)
	for s := 0; s < numSamples; s++ {
		for c, name := range leadOrder {
			offset := (s*numCh + c) * 2
			var v int16
			if samples, ok := leads[name]; ok && s < len(samples) {
				v = int16(samples[s])
			}
			binary.LittleEndian.PutUint16(buf[offset:], uint16(v))
		}
	}
	return buf
}

func findNumSamples(leads map[string][]int) int {
	max := 0
	for _, samples := range leads {
		if len(samples) > max {
			max = len(samples)
		}
	}
	return max
}

func buildStudyUID(md *mindraytofda.MindrayData) string {
	base := "1.2.840.113619.2.755"
	serial := stripDigits(md.Device.SerialNumber)
	if !md.Patient.StartTime.IsZero() {
		t := md.Patient.StartTime
		return fmt.Sprintf("%s.%s.%d%02d%02d%02d%02d%02d",
			base, serial, t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second())
	}
	return fmt.Sprintf("%s.%s", base, serial)
}

func stripDigits(s string) string {
	var out []byte
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

func dicomGender(g string) string {
	switch g {
	case "M":
		return "M"
	case "F":
		return "F"
	default:
		return "O"
	}
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

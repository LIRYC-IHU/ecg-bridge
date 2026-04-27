package nktodicom

import (
	"crypto/rand"
	"fmt"

	nktofda "converter-fda/nk-to-fda"

	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/tag"
)

// BuildDICOM constructs a DICOM dataset from NK data.
func BuildDICOM(nd *nktofda.NKData) (*dicom.Dataset, error) {
	ds := &dicom.Dataset{}

	// Generate UIDs
	studyUID := newUID()
	seriesUID := newUID()
	sopUID := newUID()

	// File Meta Information
	// Transfer Syntax: Explicit VR Little Endian (1.2.840.10008.1.2.1)
	ds.Elements = append(ds.Elements,
		mustElem(tag.TransferSyntaxUID, []string{"1.2.840.10008.1.2.1"}),
		mustElem(tag.MediaStorageSOPClassUID, []string{"1.2.840.10008.5.1.4.1.1.9.1.1"}),
		mustElem(tag.MediaStorageSOPInstanceUID, []string{sopUID}),
	)

	// SOP Common Module
	// 12-lead ECG Waveform Storage: 1.2.840.10008.5.1.4.1.1.9.1.1
	ds.Elements = append(ds.Elements,
		mustElem(tag.SOPClassUID, []string{"1.2.840.10008.5.1.4.1.1.9.1.1"}),
		mustElem(tag.SOPInstanceUID, []string{sopUID}),
	)

	// Patient Module
	fullName := nd.Patient.FamilyName
	if nd.Patient.GivenName != "" {
		fullName += "^" + nd.Patient.GivenName
	}
	ds.Elements = append(ds.Elements,
		mustElem(tag.PatientName, []string{fullName}),
		mustElem(tag.PatientID, []string{nd.Patient.PatientID}),
		mustElem(tag.PatientSex, []string{nd.Patient.Gender}),
	)

	// General Study Module
	var studyDate, studyTime string
	if !nd.Patient.RecordingAt.IsZero() {
		studyDate = nd.Patient.RecordingAt.Format("20060102")
		studyTime = nd.Patient.RecordingAt.Format("150405")
	}
	ds.Elements = append(ds.Elements,
		mustElem(tag.StudyInstanceUID, []string{studyUID}),
		mustElem(tag.StudyDate, []string{studyDate}),
		mustElem(tag.StudyTime, []string{studyTime}),
		mustElem(tag.AccessionNumber, []string{nd.Patient.PatientID}),
	)

	// General Series Module
	ds.Elements = append(ds.Elements,
		mustElem(tag.Modality, []string{"ECG"}),
		mustElem(tag.SeriesInstanceUID, []string{seriesUID}),
		mustElem(tag.SeriesNumber, []string{"1"}),
	)

	// Waveform Identification Module
	ds.Elements = append(ds.Elements,
		mustElem(tag.InstanceNumber, []string{"1"}),
	)

	// Device Module (optional)
	if nd.Patient.DeviceModel != "" {
		ds.Elements = append(ds.Elements,
			mustElem(tag.Manufacturer, []string{"Nihon Kohden"}),
			mustElem(tag.ManufacturerModelName, []string{nd.Patient.DeviceModel}),
		)
	}

	// Institution Module (optional)
	if nd.Patient.Location != "" {
		ds.Elements = append(ds.Elements,
			mustElem(tag.InstitutionName, []string{nd.Patient.Location}),
		)
	}

	// Waveform Module
	if err := addWaveformSequence(ds, nd); err != nil {
		return nil, fmt.Errorf("adding waveform sequence: %w", err)
	}

	return ds, nil
}

func addWaveformSequence(ds *dicom.Dataset, nd *nktofda.NKData) error {
	if len(nd.Leads) == 0 {
		return fmt.Errorf("no waveform data available")
	}

	// Build waveform item
	item, err := buildWaveformItem(nd)
	if err != nil {
		return fmt.Errorf("building waveform item: %w", err)
	}

	// Create WaveformSequence with one item
	wfSeq, err := dicom.NewElement(tag.WaveformSequence, [][]*dicom.Element{item})
	if err != nil {
		return fmt.Errorf("creating WaveformSequence: %w", err)
	}

	ds.Elements = append(ds.Elements, wfSeq)
	return nil
}

func buildWaveformItem(nd *nktofda.NKData) ([]*dicom.Element, error) {
	// Derive 4 augmented leads from I and II
	iii, avr, avl, avf := nktofda.DeriveLeads(nd.Leads["I"], nd.Leads["II"])

	// 12-lead order: I, II, III, aVR, aVL, aVF, V1-V6
	leadOrder := []string{"I", "II", "III", "aVR", "aVL", "aVF", "V1", "V2", "V3", "V4", "V5", "V6"}
	leadData := map[string][]int32{
		"I":   nd.Leads["I"],
		"II":  nd.Leads["II"],
		"III": iii,
		"aVR": avr,
		"aVL": avl,
		"aVF": avf,
		"V1":  nd.Leads["V1"],
		"V2":  nd.Leads["V2"],
		"V3":  nd.Leads["V3"],
		"V4":  nd.Leads["V4"],
		"V5":  nd.Leads["V5"],
		"V6":  nd.Leads["V6"],
	}

	// SCPECG codes for leads
	scpecgCodes := map[string]string{
		"I":   "5.6.3-9-1",
		"II":  "5.6.3-9-2",
		"III": "5.6.3-9-61",
		"aVR": "5.6.3-9-62",
		"aVL": "5.6.3-9-63",
		"aVF": "5.6.3-9-64",
		"V1":  "5.6.3-9-3",
		"V2":  "5.6.3-9-4",
		"V3":  "5.6.3-9-5",
		"V4":  "5.6.3-9-6",
		"V5":  "5.6.3-9-7",
		"V6":  "5.6.3-9-8",
	}

	nSamples := nd.Record.TotalSamples
	nChannels := 12

	// Build ChannelDefinitionSequence
	channelItems := make([][]*dicom.Element, 0, nChannels)
	for _, name := range leadOrder {
		ch, err := buildChannelDef(name, scpecgCodes[name], nd.Record.Scale)
		if err != nil {
			return nil, fmt.Errorf("channel %s: %w", name, err)
		}
		channelItems = append(channelItems, ch)
	}
	chanDefSeq, err := dicom.NewElement(tag.ChannelDefinitionSequence, channelItems)
	if err != nil {
		return nil, err
	}

	// Interleave waveform data: [t0_L0, t0_L1, ..., t0_L11, t1_L0, ...]
	leads := make([][]int32, nChannels)
	for i, name := range leadOrder {
		leads[i] = leadData[name]
	}
	waveformData := interleaveLeads(leads, nSamples)

	item := []*dicom.Element{
		mustElem(tag.WaveformOriginality, []string{"ORIGINAL"}),
		mustElem(tag.NumberOfWaveformChannels, []int{nChannels}),
		mustElem(tag.NumberOfWaveformSamples, []int{nSamples}),
		mustElem(tag.SamplingFrequency, []string{fmt.Sprintf("%f", float64(nd.Record.SampleRate))}),
		mustElem(tag.MultiplexGroupLabel, []string{"RHYTHM"}),
		chanDefSeq,
		mustElem(tag.WaveformBitsAllocated, []int{16}),
		mustElem(tag.WaveformSampleInterpretation, []string{"SS"}),
		mustElem(tag.WaveformData, waveformData),
	}

	// Add annotations if available
	if annotSeq, err := buildAnnotations(nd); err == nil && annotSeq != nil {
		item = append(item, annotSeq)
	}

	return item, nil
}

func buildAnnotations(nd *nktofda.NKData) (*dicom.Element, error) {
	type measurement struct {
		codeValue   string
		codeMeaning string
		value       int
		unit        string
		ucumCode    string
	}

	m := nd.Measurement
	measurements := []measurement{
		{"8867-4", "Heart Rate", m.HeartRate, "/min", "/min"},
		{"8625-3", "PR Interval", m.PRInterval, "ms", "ms"},
		{"8633-7", "QRS Duration", m.QRSDuration, "ms", "ms"},
		{"8634-5", "QT Interval", m.QTInterval, "ms", "ms"},
		{"8636-0", "QTc Interval", m.QTcInterval, "ms", "ms"},
	}

	// Add axes if available
	if m.HasPAxis {
		measurements = append(measurements, measurement{"8626-1", "P-wave Axis", m.PAxis, "deg", "deg"})
	}
	if m.HasQRSAxis {
		measurements = append(measurements, measurement{"8632-9", "QRS Axis", m.QRSAxis, "deg", "deg"})
	}
	if m.HasTAxis {
		measurements = append(measurements, measurement{"8638-7", "T-wave Axis", m.TAxis, "deg", "deg"})
	}

	var items [][]*dicom.Element
	for _, ms := range measurements {
		if ms.value == 0 {
			continue
		}
		item, err := buildAnnotationItem(ms.codeValue, ms.codeMeaning, float64(ms.value), ms.unit, ms.ucumCode)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if len(items) == 0 {
		return nil, nil
	}

	return dicom.NewElement(tag.WaveformAnnotationSequence, items)
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
			mustElem(tag.NumericValue, []string{fmt.Sprintf("%f", value)}),
			unitSeq,
		},
	})
	if err != nil {
		return nil, err
	}

	return []*dicom.Element{conceptSeq, measSeq}, nil
}

func buildChannelDef(leadName, scpecgCode string, sensitivity float64) ([]*dicom.Element, error) {
	// ChannelSourceSequence
	srcSeq, err := dicom.NewElement(tag.ChannelSourceSequence, [][]*dicom.Element{
		{
			mustElem(tag.CodeValue, []string{scpecgCode}),
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
		mustElem(tag.ChannelSensitivity, []string{fmt.Sprintf("%f", sensitivity)}),
		unitSeq,
		mustElem(tag.ChannelSensitivityCorrectionFactor, []string{"1"}),
		mustElem(tag.ChannelBaseline, []string{"0"}),
		mustElem(tag.ChannelSampleSkew, []string{"0"}),
		mustElem(tag.WaveformBitsStored, []int{16}),
	}
	return ch, nil
}

func interleaveLeads(leads [][]int32, nSamples int) []byte {
	nChannels := len(leads)
	waveformData := make([]int16, nSamples*nChannels)
	for t := 0; t < nSamples; t++ {
		for ch := 0; ch < nChannels; ch++ {
			if t < len(leads[ch]) {
				waveformData[t*nChannels+ch] = int16(leads[ch][t])
			}
		}
	}

	// Convert to little-endian bytes
	waveformBytes := make([]byte, len(waveformData)*2)
	for i, val := range waveformData {
		waveformBytes[i*2] = byte(val & 0xFF)
		waveformBytes[i*2+1] = byte((val >> 8) & 0xFF)
	}
	return waveformBytes
}

func newUID() string {
	// Generate UID using DICOM UID format: 2.25.<UUID-as-integer>
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	// Convert to decimal string
	return fmt.Sprintf("2.25.%d%d%d%d%d%d%d%d%d%d%d%d%d%d%d%d",
		b[0], b[1], b[2], b[3], b[4], b[5], b[6], b[7],
		b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15])
}

func mustElem(t tag.Tag, data any) *dicom.Element {
	el, err := dicom.NewElement(t, data)
	if err != nil {
		panic(fmt.Sprintf("failed to create element %v: %v", t, err))
	}
	return el
}

package fukudatodicom

import (
	"crypto/rand"
	"fmt"

	dicomconf "github.com/LIRYC-IHU/ecg-bridge/dicomconf"
	fukudatofda "github.com/LIRYC-IHU/ecg-bridge/fukuda-to-fda"

	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/tag"
)

// BuildDICOM constructs a 12-lead ECG DICOM dataset from Fukuda data.
func BuildDICOM(fd *fukudatofda.FukudaData) (*dicom.Dataset, error) {
	ds := &dicom.Dataset{}

	studyUID := newUID()
	seriesUID := newUID()
	sopUID := newUID()

	// File Meta / Transfer Syntax: Explicit VR Little Endian.
	ds.Elements = append(ds.Elements,
		mustElem(tag.TransferSyntaxUID, []string{"1.2.840.10008.1.2.1"}),
		mustElem(tag.MediaStorageSOPClassUID, []string{"1.2.840.10008.5.1.4.1.1.9.1.1"}),
		mustElem(tag.MediaStorageSOPInstanceUID, []string{sopUID}),
	)

	// SOP Common — 12-lead ECG Waveform Storage.
	ds.Elements = append(ds.Elements,
		mustElem(tag.SOPClassUID, []string{"1.2.840.10008.5.1.4.1.1.9.1.1"}),
		mustElem(tag.SOPInstanceUID, []string{sopUID}),
	)

	// Patient Module.
	fullName := fd.Patient.FamilyName
	if fd.Patient.GivenName != "" {
		fullName += "^" + fd.Patient.GivenName
	}
	ds.Elements = append(ds.Elements,
		mustElem(tag.PatientName, []string{fullName}),
		mustElem(tag.PatientID, []string{fd.Patient.PatientID}),
		mustElem(tag.PatientSex, []string{fd.Patient.Gender}),
	)

	// General Study Module.
	var studyDate, studyTime string
	if !fd.Patient.RecordingAt.IsZero() {
		studyDate = fd.Patient.RecordingAt.Format("20060102")
		studyTime = fd.Patient.RecordingAt.Format("150405")
	}
	ds.Elements = append(ds.Elements,
		mustElem(tag.StudyInstanceUID, []string{studyUID}),
		mustElem(tag.StudyDate, []string{studyDate}),
		mustElem(tag.StudyTime, []string{studyTime}),
		mustElem(tag.AccessionNumber, []string{fd.Patient.PatientID}),
	)

	// General Series Module.
	ds.Elements = append(ds.Elements,
		mustElem(tag.Modality, []string{"ECG"}),
		mustElem(tag.SeriesInstanceUID, []string{seriesUID}),
		mustElem(tag.SeriesNumber, []string{"1"}),
	)

	// Waveform Identification Module.
	ds.Elements = append(ds.Elements,
		mustElem(tag.InstanceNumber, []string{"1"}),
	)

	// Device Module.
	model := fd.Patient.DeviceModel
	if model == "" {
		model = "FX-ECG"
	}
	ds.Elements = append(ds.Elements,
		mustElem(tag.Manufacturer, []string{"Fukuda Denshi"}),
		mustElem(tag.ManufacturerModelName, []string{model}),
	)

	if fd.Patient.Location != "" {
		ds.Elements = append(ds.Elements,
			mustElem(tag.InstitutionName, []string{fd.Patient.Location}),
		)
	}

	if err := addWaveformSequence(ds, fd); err != nil {
		return nil, fmt.Errorf("adding waveform sequence: %w", err)
	}

	return ds, nil
}

func addWaveformSequence(ds *dicom.Dataset, fd *fukudatofda.FukudaData) error {
	if len(fd.Leads) == 0 {
		return fmt.Errorf("no waveform data available")
	}
	item, err := buildWaveformItem(fd)
	if err != nil {
		return fmt.Errorf("building waveform item: %w", err)
	}
	wfSeq, err := dicom.NewElement(tag.WaveformSequence, [][]*dicom.Element{item})
	if err != nil {
		return fmt.Errorf("creating WaveformSequence: %w", err)
	}
	ds.Elements = append(ds.Elements, wfSeq)
	return nil
}

func buildWaveformItem(fd *fukudatofda.FukudaData) ([]*dicom.Element, error) {
	iii, avr, avl, avf := fukudatofda.DeriveLeads(fd.Leads["I"], fd.Leads["II"])

	leadOrder := []string{"I", "II", "III", "aVR", "aVL", "aVF", "V1", "V2", "V3", "V4", "V5", "V6"}
	leadData := map[string][]int32{
		"I": fd.Leads["I"], "II": fd.Leads["II"], "III": iii,
		"aVR": avr, "aVL": avl, "aVF": avf,
		"V1": fd.Leads["V1"], "V2": fd.Leads["V2"], "V3": fd.Leads["V3"],
		"V4": fd.Leads["V4"], "V5": fd.Leads["V5"], "V6": fd.Leads["V6"],
	}

	nSamples := fd.Record.TotalSamples
	nChannels := 12

	channelItems := make([][]*dicom.Element, 0, nChannels)
	for _, name := range leadOrder {
		ch, err := buildChannelDef(name, dicomconf.SCPECGLeadCode(name), fd.Record.Scale)
		if err != nil {
			return nil, fmt.Errorf("channel %s: %w", name, err)
		}
		channelItems = append(channelItems, ch)
	}
	chanDefSeq, err := dicom.NewElement(tag.ChannelDefinitionSequence, channelItems)
	if err != nil {
		return nil, err
	}

	leads := make([][]int32, nChannels)
	for i, name := range leadOrder {
		leads[i] = leadData[name]
	}
	waveformData := interleaveLeads(leads, nSamples)

	item := []*dicom.Element{
		mustElem(tag.WaveformOriginality, []string{"ORIGINAL"}),
		mustElem(tag.NumberOfWaveformChannels, []int{nChannels}),
		mustElem(tag.NumberOfWaveformSamples, []int{nSamples}),
		mustElem(tag.SamplingFrequency, []string{fmt.Sprintf("%f", float64(fd.Record.SampleRate))}),
		mustElem(tag.MultiplexGroupLabel, []string{"RHYTHM"}),
		chanDefSeq,
		mustElem(tag.WaveformBitsAllocated, []int{16}),
		mustElem(tag.WaveformSampleInterpretation, []string{"SS"}),
		mustElem(tag.WaveformData, waveformData),
	}

	if annotSeq, err := buildAnnotations(fd); err == nil && annotSeq != nil {
		item = append(item, annotSeq)
	}
	return item, nil
}

// buildAnnotations emits the analytical measurements as a LOINC-coded
// WaveformAnnotationSequence. Returns (nil, nil) when no measurements are set.
func buildAnnotations(fd *fukudatofda.FukudaData) (*dicom.Element, error) {
	type meas struct {
		code, meaning string
		value         int
		unit          string
	}
	m := fd.Measurement
	list := []meas{
		{"8867-4", "Heart Rate", m.HeartRate, "/min"},
		{"8625-3", "PR Interval", m.PRInterval, "ms"},
		{"8633-7", "QRS Duration", m.QRSDuration, "ms"},
		{"8634-5", "QT Interval", m.QTInterval, "ms"},
		{"8636-0", "QTc Interval", m.QTcInterval, "ms"},
	}
	if m.HasQRSAxis {
		list = append(list, meas{"8632-9", "QRS Axis", m.QRSAxis, "deg"})
	}

	var items [][]*dicom.Element
	for _, ms := range list {
		if ms.value == 0 && ms.code != "8632-9" {
			continue
		}
		it, err := buildAnnotationItem(ms.code, ms.meaning, float64(ms.value), ms.unit)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	if len(items) == 0 {
		return nil, nil
	}
	return dicom.NewElement(tag.WaveformAnnotationSequence, items)
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
	samples := make([]int16, nSamples*nChannels)
	for t := 0; t < nSamples; t++ {
		for ch := 0; ch < nChannels; ch++ {
			if t < len(leads[ch]) {
				samples[t*nChannels+ch] = int16(leads[ch][t])
			}
		}
	}
	out := make([]byte, len(samples)*2)
	for i, v := range samples {
		out[i*2] = byte(v & 0xFF)
		out[i*2+1] = byte((v >> 8) & 0xFF)
	}
	return out
}

func newUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
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

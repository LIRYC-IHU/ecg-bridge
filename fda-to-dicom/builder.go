package fdatodicom

import (
	"fmt"

	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/tag"
)

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
	add(&ds, tag.ManufacturerModelName, []string{d.ModelName})
	add(&ds, tag.SoftwareVersions, []string{d.SoftwareVer})
	add(&ds, tag.PatientName, []string{d.PatientName})
	add(&ds, tag.PatientID, []string{d.PatientID})
	add(&ds, tag.PatientSex, []string{d.PatientSex})
	add(&ds, tag.PatientBirthDate, []string{d.PatientDOB})
	add(&ds, tag.StudyInstanceUID, []string{d.StudyUID})
	add(&ds, tag.SeriesInstanceUID, []string{d.StudyUID + ".1"})
	add(&ds, tag.StudyID, []string{"1"})
	add(&ds, tag.SeriesNumber, []string{"1"})
	add(&ds, tag.InstanceNumber, []string{"1"})

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

	return []*dicom.Element{conceptSeq, measSeq}, nil
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

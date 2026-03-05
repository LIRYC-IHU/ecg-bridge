package fdatodicom

import (
	"fmt"
	"strconv"
	"strings"

	hl7aecg "github.com/LIRYC-IHU/hl7v3-aecg/hl7aecg"
	"github.com/LIRYC-IHU/hl7v3-aecg/hl7aecg/types"
)

// FDAData is the unified model extracted from an FDA aECG XML file.
type FDAData struct {
	// Patient
	PatientID   string
	PatientName string
	PatientSex  string // "M" / "F" / ""
	PatientDOB  string // YYYYMMDD

	// Study
	StudyDate string // YYYYMMDD
	StudyTime string // HHMMSS
	StudyUID  string // from id root

	// Device
	Manufacturer    string
	ModelName       string
	SerialNumber    string
	SoftwareVer     string
	InstitutionName string

	// Filters (Hz)
	FilterLPF   float64
	FilterHPF   float64
	NotchFilter float64

	// Measurements
	HeartRate    float64
	PRInterval   float64
	QRSDuration  float64
	QTInterval   float64
	QTcInterval  float64
	AtrialRate   float64
	PFrontAxis   float64
	QRSFrontAxis float64
	TFrontAxis   float64
	QTDispersion float64
}

// ParseFDA reads an FDA aECG XML file and returns an FDAData.
func ParseFDA(path string) (*FDAData, error) {
	h := hl7aecg.NewHl7xml("")
	if err := h.UnmarshalFromFile(path); err != nil {
		return nil, fmt.Errorf("parsing FDA XML: %w", err)
	}

	ecg := &h.HL7AEcg
	d := &FDAData{}

	// ── Patient ───────────────────────────────────────────────────────────────
	if person := subjectDemographicPerson(ecg); person != nil {
		d.PatientID = strings.TrimSpace(person.PatientID)
		if person.Name != nil {
			d.PatientName = strings.TrimSpace(*person.Name)
		}
		if person.AdministrativeGenderCode != nil {
			d.PatientSex = mapGender(string(person.AdministrativeGenderCode.Code))
		}
		if person.BirthTime != nil {
			d.PatientDOB = hl7Date(person.BirthTime.Value)
		}
	}

	// ── Study date/time ───────────────────────────────────────────────────────
	if ecg.EffectiveTime != nil {
		d.StudyDate, d.StudyTime = splitHL7DateTime(ecg.EffectiveTime.Low.Value)
	}
	if ecg.ID != nil {
		d.StudyUID = hl7OIDtoUID(ecg.ID.Root)
	}

	// ── Series (first series) ─────────────────────────────────────────────────
	series := firstSeries(ecg)
	if series == nil {
		return d, nil
	}

	// Device
	if series.Author != nil {
		sa := series.Author.SeriesAuthor
		dev := sa.ManufacturedSeriesDevice
		if dev.ManufacturerModelName != nil {
			d.ModelName = strings.TrimSpace(*dev.ManufacturerModelName)
		}
		if dev.SerialNumber != nil {
			d.SerialNumber = strings.TrimSpace(*dev.SerialNumber)
		}
		if dev.SoftwareName != nil {
			d.SoftwareVer = strings.TrimSpace(*dev.SoftwareName)
		}
		if sa.ManufacturerOrganization != nil && sa.ManufacturerOrganization.Name != nil {
			d.Manufacturer = strings.TrimSpace(*sa.ManufacturerOrganization.Name)
		}
	}

	// Filters
	d.FilterLPF, d.FilterHPF, d.NotchFilter = parseFilters(series.ControlVariable)

	// Annotations / measurements
	for _, so := range series.SubjectOf {
		if so.AnnotationSet == nil {
			continue
		}
		for i := range so.AnnotationSet.Component {
			ann := &so.AnnotationSet.Component[i].Annotation
			extractMeasurement(d, ann)
		}
	}

	return d, nil
}

// subjectDemographicPerson navigates the hierarchy to reach the patient demographics.
func subjectDemographicPerson(ecg *types.HL7AEcg) *types.SubjectDemographicPerson {
	// Via ComponentOf > TimepointEvent > ComponentOf > SubjectAssignment > Subject
	if ecg.ComponentOf != nil {
		sa := &ecg.ComponentOf.TimepointEvent.ComponentOf.SubjectAssignment
		return sa.Subject.TrialSubject.SubjectDemographicPerson
	}
	// Direct subject
	if ecg.Subject != nil {
		return ecg.Subject.SubjectDemographicPerson
	}
	return nil
}

// firstSeries returns the first Series in Component list.
func firstSeries(ecg *types.HL7AEcg) *types.Series {
	if len(ecg.Component) == 0 {
		return nil
	}
	return &ecg.Component[0].Series
}

// parseFilters extracts LPF, HPF and notch from ControlVariable list.
func parseFilters(cvs []types.ControlVariable) (lpf, hpf, notch float64) {
	for i := range cvs {
		inner := cvs[i].ControlVariable
		if inner == nil || inner.Code == nil {
			continue
		}
		code := inner.Code.Code
		switch code {
		case "MDC_ECG_CTL_VBL_ATTR_FILTER_LOW_PASS":
			lpf = nestedCutoff(inner)
		case "MDC_ECG_CTL_VBL_ATTR_FILTER_HIGH_PASS":
			hpf = nestedCutoff(inner)
		case "MDC_ECG_CTL_VBL_ATTR_FILTER_NOTCH":
			notch = nestedNotchFreq(inner)
		}
	}
	return
}

// nestedCutoff extracts the cutoff frequency from a filter control variable.
func nestedCutoff(cv *types.ControlVariableInner) float64 {
	for i := range cv.Component {
		inner := cv.Component[i].ControlVariable
		if inner == nil || inner.Code == nil {
			continue
		}
		if inner.Code.Code == "MDC_ECG_CTL_VBL_ATTR_FILTER_CUTOFF_FREQ" && inner.Value != nil {
			f, _ := strconv.ParseFloat(strings.TrimSpace(inner.Value.Value), 64)
			return f
		}
	}
	return 0
}

// nestedNotchFreq extracts the notch frequency from a notch filter control variable.
func nestedNotchFreq(cv *types.ControlVariableInner) float64 {
	for i := range cv.Component {
		inner := cv.Component[i].ControlVariable
		if inner == nil || inner.Code == nil {
			continue
		}
		if inner.Code.Code == "MDC_ECG_CTL_VBL_ATTR_FILTER_NOTCH_FREQ" && inner.Value != nil {
			f, _ := strconv.ParseFloat(strings.TrimSpace(inner.Value.Value), 64)
			return f
		}
	}
	return 0
}

// extractMeasurement reads standard MDC measurement annotations into d.
func extractMeasurement(d *FDAData, ann *types.Annotation) {
	if ann.Code == nil {
		return
	}
	code := string(ann.Code.Code)
	val, hasVal := ann.GetValueFloat()

	switch code {
	case "MDC_ECG_HEART_RATE":
		if hasVal {
			d.HeartRate = val
		}
	case "MDC_ECG_TIME_PD_PR":
		if hasVal {
			d.PRInterval = val
		}
	case "MDC_ECG_TIME_PD_QRS":
		if hasVal {
			d.QRSDuration = val
		}
	case "MDC_ECG_TIME_PD_QT":
		if hasVal {
			d.QTInterval = val
		}
	case "MDC_ECG_TIME_PD_QTc", "MDC_ECG_TIME_PD_QTC":
		if hasVal {
			d.QTcInterval = val
		} else {
			// Value may be empty; look in nested components
			for i := range ann.Component {
				nested := &ann.Component[i].Annotation
				if v, ok := nested.GetValueFloat(); ok && d.QTcInterval == 0 {
					d.QTcInterval = v
				}
			}
		}
	}
}

// mapGender maps HL7 gender codes to DICOM values.
func mapGender(code string) string {
	switch strings.ToUpper(code) {
	case "M":
		return "M"
	case "F":
		return "F"
	default:
		return ""
	}
}

// splitHL7DateTime splits "YYYYMMDDHHmmss[.SSS]" into DICOM date and time strings.
func splitHL7DateTime(v string) (date, time string) {
	v = strings.TrimSpace(v)
	if dot := strings.IndexByte(v, '.'); dot >= 0 {
		v = v[:dot]
	}
	if len(v) >= 8 {
		date = v[:8]
	}
	if len(v) >= 14 {
		time = v[8:14]
	}
	return
}

// hl7Date strips sub-second parts from HL7 timestamps and returns YYYYMMDD.
func hl7Date(v string) string {
	v = strings.TrimSpace(strings.ReplaceAll(v, "-", ""))
	if len(v) >= 8 {
		return v[:8]
	}
	return v
}

// hl7OIDtoUID converts an HL7 OID/UUID root to a DICOM-compatible UID.
func hl7OIDtoUID(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	if strings.ContainsRune(root, '.') {
		return root
	}
	// UUID: strip hyphens, convert 128-bit hex to decimal via 2.25 prefix
	hex := strings.ReplaceAll(root, "-", "")
	if len(hex) == 32 {
		if uid := hexUUIDtoOID(hex); uid != "" {
			return uid
		}
	}
	return root
}

func hexUUIDtoOID(hex string) string {
	if len(hex) != 32 {
		return ""
	}
	hi, err1 := strconv.ParseUint(hex[:16], 16, 64)
	lo, err2 := strconv.ParseUint(hex[16:], 16, 64)
	if err1 != nil || err2 != nil {
		return ""
	}
	return "2.25." + uint128Decimal(hi, lo)
}

func uint128Decimal(hi, lo uint64) string {
	if hi == 0 {
		return strconv.FormatUint(lo, 10)
	}
	digits := make([]byte, 0, 40)
	for hi > 0 || lo > 0 {
		hiRem := hi % 10
		hi /= 10
		combined := hiRem*6 + lo%10
		lo = hiRem*1844674407370955161 + lo/10 + combined/10
		digits = append(digits, byte('0'+combined%10))
	}
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}

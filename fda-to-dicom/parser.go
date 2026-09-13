package fdatodicom

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/LIRYC-IHU/ecg-bridge/metaject"

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
	PatientAge  string // DICOM AS format e.g. "050Y"

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
	OperatorID      string

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

	// Waveforms: lead name → samples (raw ADC integers)
	// Key: "I", "II", ..., "V6"  (from MDC_ECG_LEAD_* code)
	SamplingRate float64 // Hz
	Sensitivity  float64 // µV/LSB (from SLIST_PQ scale)
	Baseline     float64 // origin
	// Samples are kept as int32. aECG digits are plain integers with no width
	// limit, and 24-bit acquisitions routinely exceed int16; narrowing here
	// used to wrap a tall QRS into an inverted spike with nothing to show for
	// it downstream.
	Leads    map[string][]int32 // ORIGINAL (rhythm)
	RepBeats map[string][]int32 // DERIVED (representative beat)

	// ECG interpretation (from the MDC_ECG_INTERPRETATION annotation block)
	InterpretationSummary    string   // overall banner (e.g. "- ECG NORMAL -")
	InterpretationComment    string   // free comment (e.g. "Unconfirmed Diagnosis")
	InterpretationStatements []string // specific findings, in document order
}

// Anonymize blanks the direct patient identifiers (name, ID, birth date)
// while keeping clinically useful fields (sex, age, study dates, measurements,
// institution).
func (d *FDAData) Anonymize() {
	d.PatientName = ""
	d.PatientID = ""
	d.PatientDOB = ""
}

// ApplyMetadata overwrites patient-identity and study-date fields from ov.
// Only fields present in ov are applied; nil fields leave the parsed value.
func (d *FDAData) ApplyMetadata(ov *metaject.Override) {
	if ov == nil {
		return
	}
	if ov.PatientID != nil {
		d.PatientID = *ov.PatientID
	}
	if ov.PatientName != nil {
		d.PatientName = *ov.PatientName
	}
	if ov.Gender != nil {
		d.PatientSex = *ov.Gender
	}
	if ov.Age != nil {
		d.PatientAge = *ov.Age
	}
	if ov.BirthDate != nil {
		d.PatientDOB = *ov.BirthDate
	}
	if ov.Datetime != nil {
		d.StudyDate, d.StudyTime = metaject.SplitDatetime(*ov.Datetime)
	}
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
		d.PatientAge = strings.TrimSpace(person.Age)
	}

	// ── Study date/time ───────────────────────────────────────────────────────
	if ecg.EffectiveTime != nil {
		d.StudyDate, d.StudyTime = splitHL7DateTime(ecg.EffectiveTime.Low.Value)
	}
	if ecg.ID != nil {
		d.StudyUID = hl7OIDtoUID(ecg.ID.Root)
	}

	// ── Institution name from clinical trial location ──────────────────────────
	if ecg.ComponentOf != nil {
		ct := &ecg.ComponentOf.TimepointEvent.ComponentOf.SubjectAssignment.ComponentOf.ClinicalTrial
		if ct.Location != nil {
			site := &ct.Location.TrialSite
			if site.Location != nil && site.Location.Name != nil {
				d.InstitutionName = strings.TrimSpace(*site.Location.Name)
			}
		}
	}

	// ── Series (first series) ─────────────────────────────────────────────────
	series := firstSeries(ecg)
	if series == nil {
		return d, nil
	}

	// Device (from first series author)
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

	// Operator (from first secondary performer)
	if len(series.SecondaryPerformer) > 0 {
		sp := &series.SecondaryPerformer[0]
		if sp.SeriesPerformer.AssignedPerson != nil && sp.SeriesPerformer.AssignedPerson.Name != nil {
			d.OperatorID = strings.TrimSpace(*sp.SeriesPerformer.AssignedPerson.Name)
		}
	}

	// Filters
	d.FilterLPF, d.FilterHPF, d.NotchFilter = parseFilters(series.ControlVariable)

	// Waveforms — iterate all top-level series
	d.Leads = make(map[string][]int32)
	d.RepBeats = make(map[string][]int32)
	for si := range ecg.Component {
		s := &ecg.Component[si].Series
		// Rhythm waveforms from this series' direct components
		for ci := range s.Component {
			ss := &s.Component[ci].SequenceSet
			sr, sens, base, leads, err := parseSequenceSet(ss)
			if err != nil {
				return nil, err
			}
			if sr > 0 && d.SamplingRate == 0 {
				d.SamplingRate = sr
			}
			if sens > 0 && d.Sensitivity == 0 {
				d.Sensitivity = sens
				d.Baseline = base
			}
			for k, v := range leads {
				d.Leads[k] = v
			}
		}
		// Representative beat waveforms from derivation
		for di := range s.Derivation {
			ds := &s.Derivation[di].DerivedSeries
			for ci := range ds.Component {
				ss := &ds.Component[ci].SequenceSet
				sr, sens, base, leads, err := parseSequenceSet(ss)
				if err != nil {
					return nil, err
				}
				if sr > 0 && d.SamplingRate == 0 {
					d.SamplingRate = sr
				}
				if sens > 0 && d.Sensitivity == 0 {
					d.Sensitivity = sens
					d.Baseline = base
				}
				for k, v := range leads {
					d.RepBeats[k] = v
				}
			}
		}
	}

	// Annotations — from all series and their derived series
	for si := range ecg.Component {
		s := &ecg.Component[si].Series
		extractSeriesAnnotations(d, s)
		for di := range s.Derivation {
			ds := &s.Derivation[di].DerivedSeries
			extractSeriesAnnotations(d, ds)
		}
	}

	return d, nil
}

// extractSeriesAnnotations reads measurement annotations from a series' SubjectOf list.
func extractSeriesAnnotations(d *FDAData, s *types.Series) {
	for _, so := range s.SubjectOf {
		if so.AnnotationSet == nil {
			continue
		}
		for i := range so.AnnotationSet.Component {
			ann := &so.AnnotationSet.Component[i].Annotation
			extractMeasurement(d, ann)
		}
	}
}

// parseSequenceSet extracts sampling rate, sensitivity, baseline and lead
// samples from a SequenceSet. Returns samplingRate=0 if no time sequence found.
//
// Every unit carried by the file is read and honoured. Nothing is assumed: an
// unrecognised unit is an error, because the alternative — treating it as the
// one we expected — produces a trace that is wrong by a factor of 1000 with
// nothing on the document to show for it.
func parseSequenceSet(ss *types.SequenceSet) (samplingRate, sensitivity, baseline float64, leads map[string][]int32, err error) {
	leads = make(map[string][]int32)
	for ci := range ss.Component {
		seq := &ss.Component[ci].Sequence
		if seq.Code.Lead == nil {
			// Time sequence — sampling rate from the increment.
			if seq.Value == nil {
				continue
			}
			// GLIST_TS and GLIST_PQ carry the increment in different structs
			// with the same two fields, so read the fields, not the type.
			var incValue, incUnit string
			switch v := seq.Value.Typed.(type) {
			case *types.GLIST_TS:
				incValue, incUnit = v.Increment.Value, v.Increment.Unit
			case *types.GLIST_PQ:
				incValue, incUnit = v.Increment.Value, v.Increment.Unit
			default:
				continue
			}
			step, perr := strconv.ParseFloat(strings.TrimSpace(incValue), 64)
			if perr != nil || step <= 0 {
				continue
			}
			seconds, uerr := incrementToSeconds(step, incUnit)
			if uerr != nil {
				return 0, 0, 0, nil, uerr
			}
			samplingRate = 1.0 / seconds
			continue
		}
		// Voltage sequence
		leadName := leadCodeToName(seq.Code.Lead.Code)
		if leadName == "" || seq.Value == nil {
			continue
		}
		var digits []int
		switch v := seq.Value.Typed.(type) {
		case *types.SLIST_PQ:
			var derr error
			digits, derr = v.GetDigits()
			if derr != nil {
				continue
			}
			sens, serr := quantityToMicrovolts(v.Scale)
			if serr != nil {
				return 0, 0, 0, nil, fmt.Errorf("lead %s scale: %w", leadName, serr)
			}
			base, berr := quantityToMicrovolts(v.Origin)
			if berr != nil {
				return 0, 0, 0, nil, fmt.Errorf("lead %s origin: %w", leadName, berr)
			}
			// aECG carries a scale per lead, and this model holds one. Taking
			// the first and ignoring the rest is only safe while they agree —
			// and the case where they do not is the documented one: precordial
			// leads recorded at half gain. Applying lead I's gain to V1..V6
			// would halve or double those amplitudes on a document that
			// declares a single calibration, with nothing to show for it.
			//
			// Refused rather than rendered. Supporting per-lead gains properly
			// means carrying them through to the renderer and stating each on
			// the document (IHE CARD TF-2 §4.6.4.2.2.4); until then, a file
			// that needs it must not be silently mis-scaled.
			if sensitivity == 0 {
				sensitivity, baseline = sens, base
			} else if sens != sensitivity {
				return 0, 0, 0, nil, fmt.Errorf("%w: lead %s is scaled at %g µV/LSB where an earlier lead is at %g µV/LSB",
					ErrPerLeadScale, leadName, sens, sensitivity)
			}
		case *types.SLIST_INT:
			var derr error
			digits, derr = v.GetDigits()
			if derr != nil {
				continue
			}
		default:
			continue
		}
		samples := make([]int32, len(digits))
		for i, d := range digits {
			if d > math.MaxInt32 || d < math.MinInt32 {
				return 0, 0, 0, nil, fmt.Errorf("lead %s: sample %d exceeds int32", leadName, d)
			}
			samples[i] = int32(d)
		}
		leads[leadName] = samples
	}
	return
}

// leadCodeToName converts a MDC_ECG_LEAD_* code to a short name like "I", "V1".
func leadCodeToName(code types.LeadCode) string {
	s := string(code)
	switch s {
	case "MDC_ECG_LEAD_I":
		return "I"
	case "MDC_ECG_LEAD_II":
		return "II"
	case "MDC_ECG_LEAD_III":
		return "III"
	case "MDC_ECG_LEAD_AVR":
		return "aVR"
	case "MDC_ECG_LEAD_AVL":
		return "aVL"
	case "MDC_ECG_LEAD_AVF":
		return "aVF"
	case "MDC_ECG_LEAD_V1":
		return "V1"
	case "MDC_ECG_LEAD_V2":
		return "V2"
	case "MDC_ECG_LEAD_V3":
		return "V3"
	case "MDC_ECG_LEAD_V4":
		return "V4"
	case "MDC_ECG_LEAD_V5":
		return "V5"
	case "MDC_ECG_LEAD_V6":
		return "V6"
	default:
		return ""
	}
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
	case "MDC_ECG_INTERPRETATION":
		extractInterpretation(d, ann)
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
	case "MDC_ECG_ANGLE_P_FRONT":
		if hasVal {
			d.PFrontAxis = val
		}
	case "MDC_ECG_ANGLE_QRS_FRONT":
		if hasVal {
			d.QRSFrontAxis = val
		}
	case "MDC_ECG_ANGLE_T_FRONT":
		if hasVal {
			d.TFrontAxis = val
		}
	case "MDC_ECG_ATRIAL_RATE":
		if hasVal {
			d.AtrialRate = val
		}
	}
}

// extractInterpretation reads the nested text annotations of an
// MDC_ECG_INTERPRETATION block (summary, comment, statements) into d.
func extractInterpretation(d *FDAData, ann *types.Annotation) {
	for i := range ann.Component {
		nested := &ann.Component[i].Annotation
		if nested.Code == nil || nested.Value == nil {
			continue
		}
		txt, ok := nested.Value.GetText()
		if !ok {
			continue
		}
		if txt = strings.TrimSpace(txt); txt == "" {
			continue
		}
		switch string(nested.Code.Code) {
		case "MDC_ECG_INTERPRETATION_STATEMENT":
			d.InterpretationStatements = append(d.InterpretationStatements, txt)
		case "MDC_ECG_INTERPRETATION_SUMMARY":
			d.InterpretationSummary = txt
		case "MDC_ECG_INTERPRETATION_COMMENT":
			d.InterpretationComment = txt
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

// incrementToSeconds converts a time-sequence increment to seconds. aECG marks
// the unit explicitly; an unmarked or unknown one is refused rather than
// guessed, since a wrong time base rescales every interval on the document.
func incrementToSeconds(value float64, unit string) (float64, error) {
	switch strings.TrimSpace(unit) {
	case "s":
		return value, nil
	case "ms":
		return value / 1000.0, nil
	case "us", "µs":
		return value / 1e6, nil
	default:
		return 0, fmt.Errorf("unsupported time increment unit %q", unit)
	}
}

// ErrPerLeadScale is returned when a document scales its leads differently.
// The model here holds one amplitude scale for the whole recording, so such a
// document cannot be represented faithfully and is refused.
var ErrPerLeadScale = errors.New("fda-to-dicom: per-lead amplitude scales")

// quantityToMicrovolts reads a voltage PhysicalQuantity and returns it in µV,
// which is the unit the rest of this package works in. An absent or unknown
// unit is an error: the common case is uV, and silently assuming it turns a
// file written in mV into a trace 1000x too small.
func quantityToMicrovolts(pq types.PhysicalQuantity) (float64, error) {
	raw := strings.TrimSpace(pq.Value)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("unparseable value %q", pq.Value)
	}
	if value == 0 {
		return 0, nil
	}
	switch strings.TrimSpace(pq.Unit) {
	case "uV", "µV", "UV":
		return value, nil
	case "mV", "MV":
		return value * 1000.0, nil
	case "V":
		return value * 1e6, nil
	case "":
		return 0, fmt.Errorf("value %q carries no unit", pq.Value)
	default:
		return 0, fmt.Errorf("unsupported voltage unit %q", pq.Unit)
	}
}

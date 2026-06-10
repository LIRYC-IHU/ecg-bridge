package mindraytofda

import (
	"context"
	"crypto/rand"
	stdxml "encoding/xml"
	"fmt"
	"os"
	"strings"

	"converter-fda/metaject"

	"github.com/LIRYC-IHU/hl7v3-aecg/hl7aecg"
	"github.com/LIRYC-IHU/hl7v3-aecg/hl7aecg/types"
)

// Convert parses a Mindray file and writes FDA aECG XML.
// When anonymize is true, direct patient identifiers are stripped from the output.
// When meta is non-nil, its fields overwrite the parsed metadata (injection).
func Convert(inputPath, outputPath string, anonymize bool, meta *metaject.Override) error {
	dat, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", inputPath, err)
	}

	md, err := ParseFile(dat)
	if err != nil {
		return fmt.Errorf("parsing Mindray file: %w", err)
	}

	if anonymize {
		md.Anonymize()
	}
	md.ApplyMetadata(meta)

	xmlStr, err := buildAECG(md)
	if err != nil {
		return fmt.Errorf("building FDA XML: %w", err)
	}

	if outputPath == "" {
		fmt.Print(xmlStr)
		return nil
	}
	return os.WriteFile(outputPath, []byte(xmlStr), 0644)
}

func buildAECG(md *MindrayData) (string, error) {
	h := hl7aecg.NewHl7xml("")
	h.Initialize(types.CPT_CODE_ECG_Routine, types.CPT_OID, "CPT-4", "")
	h.HL7AEcg.ConfidentialityCode = nil
	h.HL7AEcg.ReasonCode = nil

	rootID := buildRootID(md)
	h.HL7AEcg.SetRootID(rootID, "annotatedEcg")

	var startDT, endDT string
	if !md.Patient.StartTime.IsZero() {
		startDT = md.Patient.StartTime.Format("20060102150405")
	}
	if !md.Patient.EndTime.IsZero() {
		endDT = md.Patient.EndTime.Format("20060102150405")
	}
	if startDT != "" {
		h.SetEffectiveTime(startDT, endDT, nil, nil)
	}

	// Subject
	h.SetSubject(rootID, "trialSubject", types.SUBJECT_ROLE_ENROLLED)
	gender := types.GetGender(md.Patient.Gender)
	h.SetSubjectDemographics(md.Patient.Name, md.Patient.PatientID, gender, "", types.RACE_OTHER)

	sdp := h.HL7AEcg.ComponentOf.TimepointEvent.ComponentOf.SubjectAssignment.Subject.TrialSubject.SubjectDemographicPerson
	if md.Patient.Name == "" {
		sdp.Name = nil
	}
	sdp.BirthTime = nil
	if md.Patient.Paced {
		sdp.SetPaced(true)
	}

	// ClinicalTrial
	ct := &h.HL7AEcg.ComponentOf.TimepointEvent.ComponentOf.SubjectAssignment.ComponentOf.ClinicalTrial
	ct.SetID(rootID, "clinicalTrial")

	// Location
	h.SetLocation("trialSite", rootID, md.Patient.Location, "", "", "")
	h.SetResponsibleParty(rootID, "trialInvestigator", "", "", "", "")

	// Rhythm series
	if len(md.Leads) > 0 && startDT != "" {
		leads12 := buildLeadMap(md.Leads)
		h.AddRhythmSeries(startDT, endDT, nil, nil, float64(md.Record.SampleRate), leads12, 0.0, md.Record.Scale)

		lastComp := &h.HL7AEcg.Component[len(h.HL7AEcg.Component)-1]

		// Series ID
		lastComp.Series.ID = &types.ID{Root: rootID, Extension: "series"}

		// Device author
		model := md.Device.ModelName
		serial := md.Device.SerialNumber
		software := md.Device.SoftwareName
		manufacturer := md.Device.Manufacturer
		lastComp.Series.Author = &types.Author{
			SeriesAuthor: types.SeriesAuthor{
				ManufacturedSeriesDevice: types.ManufacturedSeriesDevice{
					ManufacturerModelName: &model,
					SerialNumber:          &serial,
					SoftwareName:          &software,
				},
				ManufacturerOrganization: &types.ManufacturerOrganization{
					Name: &manufacturer,
				},
			},
		}
	}

	// Annotations
	addAnnotations(h, md, startDT)

	// Validation
	vctx := types.NewValidationContext(false)
	if err := h.HL7AEcg.Validate(context.Background(), vctx); err != nil {
		return "", fmt.Errorf("validating aECG: %w", err)
	}
	if vctx.HasErrors() {
		return "", fmt.Errorf("aECG validation: %w", vctx.GetError())
	}

	data, err := stdxml.MarshalIndent(h.HL7AEcg, "", "  ")
	if err != nil {
		return "", err
	}
	return stdxml.Header + string(data), nil
}

var leadOrder = [12]string{"I", "II", "III", "aVR", "aVL", "aVF", "V1", "V2", "V3", "V4", "V5", "V6"}

var leadNameToCode = map[string]types.LeadCode{
	"I":   types.MDC_ECG_LEAD_I,
	"II":  types.MDC_ECG_LEAD_II,
	"III": types.MDC_ECG_LEAD_III,
	"aVR": types.MDC_ECG_LEAD_AVR,
	"aVL": types.MDC_ECG_LEAD_AVL,
	"aVF": types.MDC_ECG_LEAD_AVF,
	"V1":  types.MDC_ECG_LEAD_V1,
	"V2":  types.MDC_ECG_LEAD_V2,
	"V3":  types.MDC_ECG_LEAD_V3,
	"V4":  types.MDC_ECG_LEAD_V4,
	"V5":  types.MDC_ECG_LEAD_V5,
	"V6":  types.MDC_ECG_LEAD_V6,
}

func buildLeadMap(leads map[string][]int) map[types.LeadCode][]int {
	result := make(map[types.LeadCode][]int, 12)
	for name, samples := range leads {
		lc, ok := leadNameToCode[name]
		if !ok {
			continue
		}
		result[lc] = samples
	}
	return result
}

func buildRootID(md *MindrayData) string {
	deviceID := "755"
	serial := stripAlpha(md.Device.SerialNumber)
	if serial == "" {
		serial = newUUID()
	}
	dt := ""
	if !md.Patient.StartTime.IsZero() {
		t := md.Patient.StartTime
		dt = fmt.Sprintf("%d%d%d.%d%d%d", t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second())
	}
	return fmt.Sprintf("%s.%s.%s", deviceID, serial, dt)
}

func stripAlpha(s string) string {
	var b strings.Builder
	for _, c := range s {
		if c >= '0' && c <= '9' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func addAnnotations(h *hl7aecg.Hl7xml, md *MindrayData, studyDT string) {
	if len(h.HL7AEcg.Component) == 0 {
		return
	}
	series := &h.HL7AEcg.Component[len(h.HL7AEcg.Component)-1].Series
	annSet := series.InitAnnotationSet(studyDT)

	m := md.Measurement
	if m.HeartRate > 0 {
		annSet.AddHeartRate(float64(m.HeartRate))
	}
	if m.PRInterval > 0 {
		annSet.AddPRInterval(float64(m.PRInterval))
	}
	if m.QRSDuration > 0 {
		annSet.AddQRSDuration(float64(m.QRSDuration))
	}
	if m.QTInterval > 0 {
		annSet.AddQTInterval(float64(m.QTInterval))
	}
	if m.QTcInterval > 0 {
		annSet.AddQTcInterval(float64(m.QTcInterval))
	}
	if m.HasPAxis {
		annSet.AddAnnotation(string(types.MDC_ECG_ANGLE_P_FRONT), string(types.MDC_OID), float64(m.PAxis), "deg")
	}
	if m.HasQRSAxis {
		annSet.AddAnnotation(string(types.MDC_ECG_ANGLE_QRS_FRONT), string(types.MDC_OID), float64(m.QRSAxis), "deg")
	}
	if m.HasTAxis {
		annSet.AddAnnotation(string(types.MDC_ECG_ANGLE_T_FRONT), string(types.MDC_OID), float64(m.TAxis), "deg")
	}
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

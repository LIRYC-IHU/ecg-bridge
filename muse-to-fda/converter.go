package musetofda

import (
	"context"
	stdxml "encoding/xml"
	"fmt"
	"os"

	"github.com/LIRYC-IHU/hl7v3-aecg/hl7aecg"
	"github.com/LIRYC-IHU/hl7v3-aecg/hl7aecg/types"
)

// Convert parses a GE MUSE RestingECG XML file and writes FDA aECG XML.
// If outputPath is empty, output is written to stdout.
func Convert(inputPath, outputPath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", inputPath, err)
	}

	d, err := ParseMuse(data)
	if err != nil {
		return fmt.Errorf("parsing MUSE XML: %w", err)
	}

	xmlStr, err := buildAECG(d)
	if err != nil {
		return fmt.Errorf("building aECG XML: %w", err)
	}

	if outputPath == "" {
		fmt.Print(xmlStr)
		return nil
	}
	return os.WriteFile(outputPath, []byte(xmlStr), 0644)
}

// placeholderTime is used as effectiveTime when the MUSE file carries no
// acquisition date (anonymized exports). The value is intentionally obvious.
const placeholderTime = "19000101000000"

// leadOrder maps the 12-lead output index to its FDA LeadCode.
var leadOrder = [12]types.LeadCode{
	types.MDC_ECG_LEAD_I, types.MDC_ECG_LEAD_II, types.MDC_ECG_LEAD_III,
	types.MDC_ECG_LEAD_AVR, types.MDC_ECG_LEAD_AVL, types.MDC_ECG_LEAD_AVF,
	types.MDC_ECG_LEAD_V1, types.MDC_ECG_LEAD_V2, types.MDC_ECG_LEAD_V3,
	types.MDC_ECG_LEAD_V4, types.MDC_ECG_LEAD_V5, types.MDC_ECG_LEAD_V6,
}

func buildAECG(d *MuseData) (string, error) {
	h := hl7aecg.NewHl7xml("")

	h.Initialize(types.CPT_CODE_ECG_Routine, types.CPT_OID, "CPT-4", "")
	h.HL7AEcg.SetRootID(d.StudyUID, "annotatedEcg")
	h.HL7AEcg.ConfidentialityCode = nil
	h.HL7AEcg.ReasonCode = nil

	// FDA aECG requires an effectiveTime. Anonymized MUSE exports carry no
	// acquisition date, so fall back to a clearly-synthetic placeholder.
	studyDT := d.StudyDate + d.StudyTime
	if studyDT == "" {
		studyDT = placeholderTime
		fmt.Fprintf(os.Stderr, "warning: no acquisition date in MUSE file, using placeholder %s\n", placeholderTime)
	}
	h.SetEffectiveTime(studyDT, studyDT, nil, nil)

	// Subject / demographics (mostly empty in anonymized MUSE exports).
	h.SetSubject(d.StudyUID, "trialSubject", types.SUBJECT_ROLE_ENROLLED)
	h.SetSubjectDemographics(
		d.PatientName,
		d.PatientID,
		types.GetGender(d.PatientSex),
		"", // no birth date
		types.RACE_OTHER,
	)

	sdp := h.HL7AEcg.ComponentOf.TimepointEvent.ComponentOf.SubjectAssignment.Subject.TrialSubject.SubjectDemographicPerson
	if d.PatientName == "" {
		sdp.Name = nil
	}
	sdp.BirthTime = nil
	if d.PatientAge != "" {
		sdp.SetAge(d.PatientAge)
	}

	ct := &h.HL7AEcg.ComponentOf.TimepointEvent.ComponentOf.SubjectAssignment.ComponentOf.ClinicalTrial
	ct.SetID(d.StudyUID, "clinicalTrial")
	if studyDT != "" {
		ct.SetActivityTime(studyDT, studyDT)
	}

	h.SetLocation("trialSite", d.StudyUID, "", "", "", "")
	h.SetResponsibleParty(d.StudyUID, "trialInvestigator", "", "", "", "")

	// Rhythm series (ORIGINAL) — 10 s, 12 leads.
	rhythmLeads := buildLeadMap(d.RhythmLeads)
	if len(rhythmLeads) > 0 {
		h.AddRhythmSeries(studyDT, studyDT, nil, nil, d.SamplingRate, rhythmLeads, d.Baseline, d.Sensitivity)

		// Device author (MUSE software version) on the rhythm series.
		manufacturer := "GE Healthcare"
		model := "MUSE"
		software := d.MuseVersion
		serial := ""
		lastComp := &h.HL7AEcg.Component[len(h.HL7AEcg.Component)-1]
		lastComp.Series.Author = &types.Author{
			SeriesAuthor: types.SeriesAuthor{
				ManufacturedSeriesDevice: types.ManufacturedSeriesDevice{
					Code:                  types.NewCode(types.GetDeviceTypeCode(model), types.CodeSystemOID(""), "", ""),
					ManufacturerModelName: &model,
					SerialNumber:          &serial,
					SoftwareName:          &software,
				},
				ManufacturerOrganization: &types.ManufacturerOrganization{
					Name: &manufacturer,
				},
			},
		}

		// Filters.
		if d.FilterLPF > 0 {
			h.AddLowPassFilter(fmt.Sprintf("%g", d.FilterLPF), "Hz")
		}
		if d.FilterHPF > 0 {
			h.AddHighPassFilter(fmt.Sprintf("%g", d.FilterHPF), "Hz")
		}
		if d.NotchFilter > 0 {
			h.AddNotchFilter(fmt.Sprintf("%g", d.NotchFilter), "Hz")
		}
	}

	// Median series (DERIVED, representative/median beat).
	medianLeads := buildLeadMap(d.MedianLeads)
	if len(medianLeads) > 0 && len(h.HL7AEcg.Component) > 0 {
		h.AddDerivedSeries(types.MEDIAN_BEAT_CODE, studyDT, studyDT, nil, nil, d.SamplingRate, medianLeads, d.Baseline, d.Sensitivity)
	}

	// Annotations on the rhythm series.
	addAnnotations(h, d, studyDT)

	vctx := types.NewValidationContext(false)
	if err := h.HL7AEcg.Validate(context.Background(), vctx); err != nil {
		return "", fmt.Errorf("validating aECG: %w", err)
	}
	if vctx.HasErrors() {
		return "", fmt.Errorf("aECG validation failed: %w", vctx.GetError())
	}

	out, err := stdxml.MarshalIndent(h.HL7AEcg, "", "  ")
	if err != nil {
		return "", err
	}
	return stdxml.Header + string(out), nil
}

func buildLeadMap(arr [12][]int16) map[types.LeadCode][]int {
	leads := make(map[types.LeadCode][]int, 12)
	for i, lc := range leadOrder {
		samples := arr[i]
		if len(samples) == 0 {
			continue
		}
		intSamples := make([]int, len(samples))
		for j, s := range samples {
			intSamples[j] = int(s)
		}
		leads[lc] = intSamples
	}
	return leads
}

func addAnnotations(h *hl7aecg.Hl7xml, d *MuseData, studyDT string) {
	if len(h.HL7AEcg.Component) == 0 {
		return
	}
	series := &h.HL7AEcg.Component[len(h.HL7AEcg.Component)-1].Series
	annSet := series.InitAnnotationSet(studyDT)

	if d.HeartRate > 0 {
		annSet.AddHeartRate(d.HeartRate)
	}
	if d.AtrialRate > 0 {
		annSet.AddAtrialRate(d.AtrialRate)
	}
	if d.PRInterval > 0 {
		annSet.AddPRInterval(d.PRInterval)
	}
	if d.QRSDuration > 0 {
		annSet.AddQRSDuration(d.QRSDuration)
	}
	if d.QTInterval > 0 {
		annSet.AddQTInterval(d.QTInterval)
	}
	if d.QTcInterval > 0 {
		annSet.AddQTcInterval(d.QTcInterval)
	}
	if d.PFrontAxis != 0 {
		annSet.AddAnnotation(string(types.MDC_ECG_ANGLE_P_FRONT), string(types.MDC_OID), d.PFrontAxis, "deg")
	}
	if d.QRSFrontAxis != 0 {
		annSet.AddAnnotation(string(types.MDC_ECG_ANGLE_QRS_FRONT), string(types.MDC_OID), d.QRSFrontAxis, "deg")
	}
	if d.TFrontAxis != 0 {
		annSet.AddAnnotation(string(types.MDC_ECG_ANGLE_T_FRONT), string(types.MDC_OID), d.TFrontAxis, "deg")
	}

	// Diagnosis / interpretation block.
	if len(d.DiagnosisStatements) > 0 {
		interpIdx := annSet.AddTextAnnotation("MDC_ECG_INTERPRETATION", string(types.MDC_OID), "")
		interpAnn := annSet.GetAnnotation(interpIdx)
		for _, stmt := range d.DiagnosisStatements {
			interpAnn.AddNestedTextAnnotation("MDC_ECG_INTERPRETATION_STATEMENT", string(types.MDC_OID), stmt)
		}
	}
}

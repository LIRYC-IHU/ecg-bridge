package philipstofda

import (
	"context"
	stdxml "encoding/xml"
	"fmt"
	"os"

	philipstodicom "converter-fda/philips-to-dicom"

	"github.com/LIRYC-IHU/hl7v3-aecg/hl7aecg"
	"github.com/LIRYC-IHU/hl7v3-aecg/hl7aecg/types"
)

// Convert parses a Philips SierraECG XML file and writes FDA aECG XML.
// If outputPath is empty, output is written to stdout.
func Convert(inputPath, outputPath string) error {
	data, err := philipstodicom.ParsePhilips(inputPath)
	if err != nil {
		return fmt.Errorf("parsing Philips XML: %w", err)
	}

	xmlStr, err := buildAECG(data)
	if err != nil {
		return fmt.Errorf("building aECG XML: %w", err)
	}

	if outputPath == "" {
		fmt.Print(xmlStr)
		return nil
	}

	return os.WriteFile(outputPath, []byte(xmlStr), 0644)
}

// leadOrder maps the 12-lead index (same as philips-to-dicom) to LeadCode.
var leadOrder = [12]types.LeadCode{
	types.MDC_ECG_LEAD_I, types.MDC_ECG_LEAD_II, types.MDC_ECG_LEAD_III,
	types.MDC_ECG_LEAD_AVR, types.MDC_ECG_LEAD_AVL, types.MDC_ECG_LEAD_AVF,
	types.MDC_ECG_LEAD_V1, types.MDC_ECG_LEAD_V2, types.MDC_ECG_LEAD_V3,
	types.MDC_ECG_LEAD_V4, types.MDC_ECG_LEAD_V5, types.MDC_ECG_LEAD_V6,
}

// repBeatNames maps Philips repbeat leadname to LeadCode.
var repBeatNames = map[string]types.LeadCode{
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

func buildAECG(d *philipstodicom.PhilipsData) (string, error) {
	h := hl7aecg.NewHl7xml("")

	h.Initialize(types.CPT_CODE_ECG_Routine, types.CPT_OID, "CPT-4", "")
	h.HL7AEcg.SetRootID(d.StudyUID, "")
	h.HL7AEcg.ConfidentialityCode = nil
	h.HL7AEcg.ReasonCode = nil

	studyDT := d.StudyDate + d.StudyTime
	if studyDT != "" {
		h.SetEffectiveTime(studyDT, studyDT, nil, nil)
	}

	h.SetSubject(d.StudyUID, "trialSubject", types.SUBJECT_ROLE_ENROLLED)
	h.SetSubjectDemographics(
		d.PatientName,
		d.PatientID,
		types.GetGender(d.PatientSex),
		"", // no birth date in Philips XML
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
	if d.Room != "" {
		sdp.SetRoom(d.Room)
	}

	ct := &h.HL7AEcg.ComponentOf.TimepointEvent.ComponentOf.SubjectAssignment.ComponentOf.ClinicalTrial
	ct.SetID(d.StudyUID, "clinicalTrial")
	if studyDT != "" {
		ct.SetActivityTime(studyDT, studyDT)
	}

	h.SetLocation("trialSite", d.StudyUID, d.InstitutionName, "", "", "")
	h.SetResponsibleParty(d.StudyUID, "trialInvestigator", "", d.OperatorID, "", "")

	// Rhythm series (ORIGINAL)
	rhythmLeads := buildRhythmLeads(d)
	if len(rhythmLeads) > 0 {
		h.AddRhythmSeries(studyDT, studyDT, nil, nil, d.SamplingRate, rhythmLeads, d.Baseline, d.Sensitivity)

		// Device author attached to the rhythm series
		if d.ModelName != "" {
			model := d.ModelName
			serial := ""
			software := d.SoftwareVer
			manufacturer := d.Manufacturer
			lastComp := &h.HL7AEcg.Component[len(h.HL7AEcg.Component)-1]
			lastComp.Series.Author = &types.Author{
				SeriesAuthor: types.SeriesAuthor{
					ID: &types.ID{Root: serial},
					ManufacturedSeriesDevice: types.ManufacturedSeriesDevice{
						ID:                    &types.ID{Extension: serial},
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
		}

		// Filters
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

	// Representative beat series (DERIVED)
	repLeads := buildRepBeatsLeads(d)
	if len(repLeads) > 0 {
		h.AddDerivedSeries(types.REPRESENTATIVE_BEAT_CODE, studyDT, studyDT, nil, nil, d.SamplingRate, repLeads, d.Baseline, d.Sensitivity)
	}

	// Annotations
	addAnnotations(h, d, studyDT)

	vctx := types.NewValidationContext(false)
	if err := h.HL7AEcg.Validate(context.Background(), vctx); err != nil {
		return "", fmt.Errorf("validating aECG: %w", err)
	}
	if vctx.HasErrors() {
		return "", fmt.Errorf("aECG validation failed: %w", vctx.GetError())
	}

	data, err := stdxml.MarshalIndent(h.HL7AEcg, "", "  ")
	if err != nil {
		return "", err
	}
	return stdxml.Header + string(data), nil
}

func buildRhythmLeads(d *philipstodicom.PhilipsData) map[types.LeadCode][]int {
	leads := make(map[types.LeadCode][]int)
	for i, lc := range leadOrder {
		samples := d.RhythmLeads[i]
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

func buildRepBeatsLeads(d *philipstodicom.PhilipsData) map[types.LeadCode][]int {
	if len(d.RepBeats) == 0 {
		return nil
	}
	leads := make(map[types.LeadCode][]int)
	for name, samples := range d.RepBeats {
		lc, ok := repBeatNames[name]
		if !ok {
			lc = types.NormalizeLeadCode(name)
		}
		intSamples := make([]int, len(samples))
		for j, s := range samples {
			intSamples[j] = int(s)
		}
		leads[lc] = intSamples
	}
	return leads
}

func addAnnotations(h *hl7aecg.Hl7xml, d *philipstodicom.PhilipsData, studyDT string) {
	if len(h.HL7AEcg.Component) == 0 {
		return
	}

	series := &h.HL7AEcg.Component[len(h.HL7AEcg.Component)-1].Series
	annSet := series.InitAnnotationSet(studyDT)

	if d.HeartRate > 0 {
		annSet.AddHeartRate(d.HeartRate)
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
	if d.AtrialRate > 0 {
		annSet.AddAnnotation("MDC_ECG_ATRIAL_RATE", string(types.MDC_OID), d.AtrialRate, "bpm")
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
	if d.QTDispersion > 0 {
		annSet.AddAnnotation("MDC_ECG_TIME_PD_QT_DISPERSION", string(types.MDC_OID), d.QTDispersion, "ms")
	}
}

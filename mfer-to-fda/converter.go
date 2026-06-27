package mfertofda

import (
	"context"
	"crypto/rand"
	stdxml "encoding/xml"
	"fmt"
	"os"

	"github.com/LIRYC-IHU/ecg-bridge/metaject"

	"github.com/LIRYC-IHU/hl7v3-aecg/hl7aecg"
	"github.com/LIRYC-IHU/hl7v3-aecg/hl7aecg/types"
)

// Convert parses an MFER (.mwf) file and writes FDA aECG XML to outputPath.
// If outputPath is empty, output is written to stdout.
// When anonymize is true, direct patient identifiers are stripped from the output.
// When meta is non-nil, its fields overwrite the parsed metadata (injection).
func Convert(inputPath, outputPath string, anonymize bool, meta *metaject.Override) error {
	dat, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", inputPath, err)
	}

	d, err := ParseFile(dat)
	if err != nil {
		return fmt.Errorf("parsing MFER file: %w", err)
	}

	if anonymize {
		d.Anonymize()
	}
	d.ApplyMetadata(meta)

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

// placeholderTime is used as effectiveTime when the MFER file carries no
// acquisition date (FDA aECG requires one).
const placeholderTime = "19000101000000"

// leadOrder maps the 12-lead output index to its FDA LeadCode.
var leadOrder = [12]types.LeadCode{
	types.MDC_ECG_LEAD_I, types.MDC_ECG_LEAD_II, types.MDC_ECG_LEAD_III,
	types.MDC_ECG_LEAD_AVR, types.MDC_ECG_LEAD_AVL, types.MDC_ECG_LEAD_AVF,
	types.MDC_ECG_LEAD_V1, types.MDC_ECG_LEAD_V2, types.MDC_ECG_LEAD_V3,
	types.MDC_ECG_LEAD_V4, types.MDC_ECG_LEAD_V5, types.MDC_ECG_LEAD_V6,
}

func buildAECG(d *MferData) (string, error) {
	h := hl7aecg.NewHl7xml("")

	h.Initialize(types.CPT_CODE_ECG_Routine, types.CPT_OID, "CPT-4", "")
	rootUUID := newUUID()
	if d.StudyUID == "" {
		d.StudyUID = rootUUID
	}
	h.HL7AEcg.SetRootID(d.StudyUID, "annotatedEcg")
	h.HL7AEcg.ConfidentialityCode = nil
	h.HL7AEcg.ReasonCode = nil

	studyDT := d.StudyDate + d.StudyTime
	if studyDT == "" {
		studyDT = placeholderTime
		fmt.Fprintf(os.Stderr, "warning: no acquisition date in MFER file, using placeholder %s\n", placeholderTime)
	}
	h.SetEffectiveTime(studyDT, studyDT, nil, nil)

	// Subject / demographics (often empty: NK .mwf carries no patient identity).
	h.SetSubject(d.StudyUID, "trialSubject", types.SUBJECT_ROLE_ENROLLED)
	h.SetSubjectDemographics(
		d.PatientName,
		d.PatientID,
		types.GetGender(d.PatientSex),
		d.BirthDate,
		types.RACE_OTHER,
	)

	sdp := h.HL7AEcg.ComponentOf.TimepointEvent.ComponentOf.SubjectAssignment.Subject.TrialSubject.SubjectDemographicPerson
	if d.PatientName == "" {
		sdp.Name = nil
	}
	if d.BirthDate == "" {
		sdp.BirthTime = nil
	}

	ct := &h.HL7AEcg.ComponentOf.TimepointEvent.ComponentOf.SubjectAssignment.ComponentOf.ClinicalTrial
	ct.SetID(d.StudyUID, "clinicalTrial")
	if studyDT != "" {
		ct.SetActivityTime(studyDT, studyDT)
	}

	h.SetLocation("trialSite", d.StudyUID, "", "", "", "")
	h.SetResponsibleParty(d.StudyUID, "trialInvestigator", "", "", "", "")

	// Rhythm series (ORIGINAL).
	rhythmLeads := buildLeadMap(d.RhythmLeads)
	if len(rhythmLeads) > 0 {
		h.AddRhythmSeries(studyDT, studyDT, nil, nil, d.SampleRate, rhythmLeads, 0.0, d.Scale)

		// Device author (Nihon Kohden + model + version).
		if d.ModelName != "" || d.Manufacturer != "" {
			model := d.ModelName
			manufacturer := d.Manufacturer
			software := d.SoftwareVer
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
		}

		// Filters.
		if d.FilterHPF > 0 {
			h.AddHighPassFilter(fmt.Sprintf("%g", d.FilterHPF), "Hz")
		}
		if d.NotchFilter > 0 {
			h.AddNotchFilter(fmt.Sprintf("%g", d.NotchFilter), "Hz")
		}
	}

	// Median series (DERIVED, representative beat).
	medianLeads := buildLeadMap(d.MedianLeads)
	if len(medianLeads) > 0 && len(h.HL7AEcg.Component) > 0 {
		h.AddDerivedSeries(types.MEDIAN_BEAT_CODE, studyDT, studyDT, nil, nil, d.SampleRate, medianLeads, 0.0, d.Scale)
	}

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

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

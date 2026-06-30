package dicomtofda

import (
	"context"
	stdxml "encoding/xml"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/LIRYC-IHU/hl7v3-aecg/hl7aecg"
	"github.com/LIRYC-IHU/hl7v3-aecg/hl7aecg/types"
)

// Convert parses the DICOM file, optionally merges metadata, and writes FDA aECG XML.
// If outputPath is empty, output is written to stdout.
func Convert(inputPath, outputPath, metadataPath string) error {
	data, err := ParseDicom(inputPath)
	if err != nil {
		return fmt.Errorf("reading DICOM: %w", err)
	}

	if metadataPath != "" {
		meta, err := LoadMetadata(metadataPath)
		if err != nil {
			return fmt.Errorf("reading metadata: %w", err)
		}
		data.MergeMetadata(meta)
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

func buildAECG(d *DicomData) (string, error) {
	h := hl7aecg.NewHl7xml("")

	// Procedure code
	h.Initialize(types.CPT_CODE_ECG_Routine, types.CPT_OID, "CPT-4", "")

	// Register the StudyInstanceUID as the singleton root ID — this lets the
	// library's validator auto-complete any empty ID.Root fields and satisfies
	// the top-level document ID requirement.
	h.HL7AEcg.SetRootID(d.StudyInstanceUID, "")

	// ConfidentialityCode and ReasonCode are initialized as empty by NewHl7xml
	// but are optional clinical trial fields we don't know from DICOM — nil them
	// so the validator skips them and they don't appear in the output.
	h.HL7AEcg.ConfidentialityCode = nil
	h.HL7AEcg.ReasonCode = nil

	// Effective time from study date/time
	studyDT := formatDateTime(d.StudyDate, d.StudyTime)
	if studyDT != "" {
		h.SetEffectiveTime(studyDT, studyDT, nil, nil)
	}

	// SetSubject initializes the full subject tree (ID, role, medications, classifications)
	h.SetSubject(d.StudyInstanceUID, "trialSubject", types.SUBJECT_ROLE_ENROLLED)

	// Subject demographics
	h.SetSubjectDemographics(
		d.Patient.PatientName,
		d.Patient.PatientID,
		types.GetGender(d.Patient.PatientSex),
		d.Patient.PatientBirthDate,
		types.RACE_OTHER,
	)

	// Additional demographic fields
	sdp := h.HL7AEcg.ComponentOf.TimepointEvent.ComponentOf.SubjectAssignment.Subject.TrialSubject.SubjectDemographicPerson
	// SetSubjectDemographics unconditionally sets pointer fields — null them out when empty
	// to avoid <name></name> and <birthTime value=""/> in output.
	if d.Patient.PatientName == "" {
		sdp.Name = nil
	}
	if d.Patient.PatientBirthDate == "" {
		sdp.BirthTime = nil
	}
	if d.Patient.SecondPatientID != "" {
		sdp.SetSecondPatientID(d.Patient.SecondPatientID)
	}
	if d.Patient.PatientAge != "" {
		sdp.SetAge(d.Patient.PatientAge)
	}
	if d.Patient.Paced != "" {
		sdp.SetPaced(d.Patient.Paced == "true")
	}
	if d.Patient.Bed != "" {
		sdp.SetBed(d.Patient.Bed)
	}
	if d.Patient.Room != "" {
		sdp.SetRoom(d.Patient.Room)
	}
	if d.Patient.PointOfCare != "" {
		sdp.SetPointOfCare(d.Patient.PointOfCare)
	}
	for _, med := range d.Patient.Medications {
		sdp.AddMedication(med)
	}
	for _, cc := range d.Patient.ClinicalClassifications {
		sdp.AddClinicalClassification(cc)
	}

	// Clinical trial
	ct := &h.HL7AEcg.ComponentOf.TimepointEvent.ComponentOf.SubjectAssignment.ComponentOf.ClinicalTrial
	ct.SetID(d.StudyInstanceUID, "clinicalTrial")
	if studyDT != "" {
		ct.SetActivityTime(studyDT, studyDT)
	}

	// Trial site / investigator
	locationName := d.Patient.LocationName
	if locationName == "" {
		locationName = d.Patient.InstitutionName
	}
	h.SetLocation("trialSite", d.StudyInstanceUID, locationName, "", "", "")
	h.SetResponsibleParty(d.StudyInstanceUID, "trialInvestigator", "", d.Patient.InvestigatorName, "", "")

	// Waveforms — first ORIGINAL group → rhythm series; DERIVED groups → derived series
	var rhythmAdded bool
	for _, wf := range d.Waveforms {
		isDerived := wf.Originality == "DERIVED"
		if !isDerived && !rhythmAdded {
			if err := addWaveformSeries(h, wf, studyDT); err != nil {
				return "", err
			}
			rhythmAdded = true
			// Device author (attached to the rhythm series)
			if d.Patient.DeviceModel != "" {
				model := d.Patient.DeviceModel
				serial := d.Patient.DeviceSerial
				software := d.Patient.SoftwareVersion
				manufacturer := d.Patient.Manufacturer
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
			// ECG technician (secondary performer)
			h.AddSecondaryPerformer(types.PERFORMER_ECG_TECHNICIAN, "", "", d.Patient.OperatorsName)
			// Filters from first channel
			addFilters(h, wf)
		} else if isDerived {
			addDerivedSeries(h, wf, studyDT)
		}
	}

	// Annotations (attached to last series)
	addAnnotations(h, d.Annotations, studyDT)

	// Validate the built document
	vctx := types.NewValidationContext(false)
	if err := h.HL7AEcg.Validate(context.Background(), vctx); err != nil {
		return "", fmt.Errorf("validating aECG: %w", err)
	}
	if vctx.HasErrors() {
		return "", fmt.Errorf("aECG validation failed: %w", vctx.GetError())
	}

	// Use standard encoding/xml — the custom ECUST-XX marshaller does not
	// invoke MarshalXML on SequenceValue, leaving SLIST_PQ/GLIST_TS empty.
	data, err := stdxml.MarshalIndent(h.HL7AEcg, "", "  ")
	if err != nil {
		return "", err
	}
	return stdxml.Header + string(data), nil
}

func addWaveformSeries(h *hl7aecg.Hl7xml, wf WaveformGroup, studyDT string) error {
	leads, scale, origin := buildLeadsMap(wf)
	if len(leads) == 0 {
		return nil
	}
	h.AddRhythmSeries(studyDT, studyDT, nil, nil, wf.SamplingFrequency, leads, origin, scale)
	return nil
}

// addDerivedSeries adds a DERIVED waveform group (e.g. representative beat) to the last rhythm series.
func addDerivedSeries(h *hl7aecg.Hl7xml, wf WaveformGroup, studyDT string) {
	leads, scale, origin := buildLeadsMap(wf)
	if len(leads) == 0 {
		return
	}
	h.AddDerivedSeries(types.REPRESENTATIVE_BEAT_CODE, studyDT, studyDT, nil, nil, wf.SamplingFrequency, leads, origin, scale)
}

// addFilters adds low-pass, high-pass and notch control variables from the first channel.
func addFilters(h *hl7aecg.Hl7xml, wf WaveformGroup) {
	if len(wf.Channels) == 0 {
		return
	}
	ch := wf.Channels[0]
	// DICOM FilterHighFrequency = low-pass cutoff (passes freqs below this)
	if ch.FilterHighFrequency > 0 {
		h.AddLowPassFilter(fmt.Sprintf("%g", ch.FilterHighFrequency), "Hz")
	}
	// DICOM FilterLowFrequency = high-pass cutoff (passes freqs above this)
	if ch.FilterLowFrequency > 0 {
		h.AddHighPassFilter(fmt.Sprintf("%g", ch.FilterLowFrequency), "Hz")
	}
	if ch.NotchFilterFrequency > 0 {
		h.AddNotchFilter(fmt.Sprintf("%g", ch.NotchFilterFrequency), "Hz")
	}
}

// buildLeadsMap converts WaveformGroup channels into the map[LeadCode][]int
// expected by the library. It normalizes all channels to a common scale.
func buildLeadsMap(wf WaveformGroup) (map[types.LeadCode][]int, float64, float64) {
	samples := wf.Samples()
	leads := make(map[types.LeadCode][]int)

	// Determine common scale (use first non-zero channel sensitivity)
	commonScale := 1.0
	for _, ch := range wf.Channels {
		if ch.Sensitivity != 0 {
			commonScale = ch.Sensitivity
			break
		}
	}

	for i, ch := range wf.Channels {
		if i >= len(samples) {
			break
		}

		name := ch.SourceName
		if name == "" {
			name = ch.Label
		}
		// Strip "Lead " prefix before normalizing (e.g. "Lead I" → "I")
		name = strings.TrimPrefix(name, "Lead ")
		name = strings.TrimPrefix(name, "lead ")

		leadCode := types.NormalizeLeadCode(name)

		chSamples := samples[i]
		intSamples := make([]int, len(chSamples))

		// If this channel has a different sensitivity, rescale to commonScale
		if ch.Sensitivity != 0 && ch.Sensitivity != commonScale {
			ratio := ch.Sensitivity / commonScale
			for j, s := range chSamples {
				intSamples[j] = int(math.Round(float64(s) * ratio))
			}
		} else {
			for j, s := range chSamples {
				intSamples[j] = int(s)
			}
		}

		leads[leadCode] = intSamples
	}

	// No channel definitions — use raw samples with generic names
	if len(wf.Channels) == 0 {
		for i, chSamples := range samples {
			label := fmt.Sprintf("V%d", i+1)
			leadCode := types.NormalizeLeadCode(label)
			intSamples := make([]int, len(chSamples))
			for j, s := range chSamples {
				intSamples[j] = int(s)
			}
			leads[leadCode] = intSamples
		}
	}

	// Auto-compute missing derived leads when Lead I and Lead II are present.
	// Einthoven's triangle:  III  = II - I
	// Goldberger augmented:  aVR  = -(I + II) / 2
	//                        aVL  = (2·I  - II) / 2
	//                        aVF  = (2·II - I)  / 2
	leadI, hasI := leads[types.MDC_ECG_LEAD_I]
	leadII, hasII := leads[types.MDC_ECG_LEAD_II]
	if hasI && hasII {
		n := len(leadI)
		if _, ok := leads[types.MDC_ECG_LEAD_III]; !ok {
			s := make([]int, n)
			for i := range s {
				s[i] = leadII[i] - leadI[i]
			}
			leads[types.MDC_ECG_LEAD_III] = s
		}
		if _, ok := leads[types.MDC_ECG_LEAD_AVR]; !ok {
			s := make([]int, n)
			for i := range s {
				s[i] = int(math.Round(float64(-leadI[i]-leadII[i]) / 2.0))
			}
			leads[types.MDC_ECG_LEAD_AVR] = s
		}
		if _, ok := leads[types.MDC_ECG_LEAD_AVL]; !ok {
			s := make([]int, n)
			for i := range s {
				s[i] = int(math.Round(float64(2*leadI[i]-leadII[i]) / 2.0))
			}
			leads[types.MDC_ECG_LEAD_AVL] = s
		}
		if _, ok := leads[types.MDC_ECG_LEAD_AVF]; !ok {
			s := make([]int, n)
			for i := range s {
				s[i] = int(math.Round(float64(2*leadII[i]-leadI[i]) / 2.0))
			}
			leads[types.MDC_ECG_LEAD_AVF] = s
		}
	}

	return leads, commonScale, 0.0
}

// addAnnotations adds WaveformAnnotations to the last series annotation set.
func addAnnotations(h *hl7aecg.Hl7xml, annotations []WaveformAnnotation, studyDT string) {
	if len(h.HL7AEcg.Component) == 0 || len(annotations) == 0 {
		return
	}

	series := &h.HL7AEcg.Component[len(h.HL7AEcg.Component)-1].Series
	annSet := series.InitAnnotationSet(studyDT)

	for _, ann := range annotations {
		if ann.TextValue != "" {
			idx := annSet.AddTextAnnotation(ann.Code, ann.CodeSystem, "")
			if a := annSet.GetAnnotation(idx); a != nil {
				a.AddNestedTextAnnotation(ann.Code, ann.CodeSystem, ann.TextValue)
			}
			continue
		}

		if ann.NumericValue == "" {
			continue
		}

		val := parseFloat(ann.NumericValue)
		name := strings.ToLower(ann.ConceptName)

		switch {
		case strings.Contains(name, "heart rate"):
			annSet.AddHeartRate(val)
		case strings.Contains(name, "pr interval") || strings.Contains(name, "pr duration"):
			annSet.AddPRInterval(val)
		case strings.Contains(name, "qrs"):
			annSet.AddQRSDuration(val)
		case strings.Contains(name, "qtc"):
			annSet.AddQTcInterval(val)
		case strings.Contains(name, "qt interval"):
			annSet.AddQTInterval(val)
		default:
			annSet.AddAnnotation(ann.Code, ann.CodeSystem, val, ann.Unit)
		}
	}
}

func formatDateTime(date, time string) string {
	date = strings.TrimSpace(date)
	time = strings.TrimSpace(strings.ReplaceAll(time, ":", ""))
	if date == "" {
		return ""
	}
	if time == "" {
		return date
	}
	// Trim sub-seconds if present, pad to 14 chars
	if len(time) > 6 {
		time = time[:6]
	}
	return date + time
}

package nktofda

import (
	"context"
	"crypto/rand"
	stdxml "encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/LIRYC-IHU/hl7v3-aecg/hl7aecg"
	"github.com/LIRYC-IHU/hl7v3-aecg/hl7aecg/types"
)

// Convert parses a NK .DAT file and writes FDA aECG XML to outputPath.
// If outputPath is empty, output is written to stdout.
func Convert(inputPath, outputPath string) error {
	dat, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", inputPath, err)
	}

	nd, err := ParseFile(dat)
	if err != nil {
		return fmt.Errorf("parsing NK file: %w", err)
	}

	// Decode waveforms
	secs, err := parseSections(dat)
	if err != nil {
		return err
	}
	recSec := secs[secRecord]
	// Pass data from the section start to end-of-file so that bitstreams that
	// spill one byte past the nominal section boundary (last frame of V6) can
	// still be read without a bounds panic or premature zero-fill.
	recData := dat[recSec.offset+pecHeaderSize:]
	// The median-beat templates (for QRS-zone reconstruction) are located by
	// scanning the whole file, so they are derived here rather than inside the
	// RECORD-only decoder.
	avg := buildAvgTemplates(dat)
	leads, err := DecodeLeads(recData, nd.Record.TotalSamples, avg)
	if err != nil {
		return fmt.Errorf("decoding waveforms: %w", err)
	}
	nd.Leads = leads

	baseName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))

	xmlStr, err := buildAECG(nd, baseName)
	if err != nil {
		return fmt.Errorf("building FDA XML: %w", err)
	}

	if outputPath == "" {
		fmt.Print(xmlStr)
		return nil
	}
	return os.WriteFile(outputPath, []byte(xmlStr), 0644)
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func buildAECG(nd *NKData, baseName string) (string, error) {
	h := hl7aecg.NewHl7xml("")
	h.Initialize(types.CPT_CODE_ECG_Routine, types.CPT_OID, "CPT-4", "")
	h.HL7AEcg.ConfidentialityCode = nil
	h.HL7AEcg.ReasonCode = nil

	rootUUID := newUUID()
	h.HL7AEcg.SetRootID(rootUUID, "")

	dt := nd.Patient.RecordingAt
	var startDT, endDT string
	if !dt.IsZero() {
		startDT = dt.Format("20060102150405")
		dur := recordingDuration(nd.Record.TotalSamples, nd.Record.SampleRate)
		endDT = dt.Add(dur).Format("20060102150405")
		h.SetEffectiveTime(startDT, startDT, nil, nil)
	}

	// Subject and demographics
	h.SetSubject(rootUUID, nd.Patient.PatientID, types.SUBJECT_ROLE_ENROLLED)
	fullName := strings.TrimSpace(nd.Patient.FamilyName + " " + nd.Patient.GivenName)
	gender := types.GetGender(nd.Patient.Gender)
	h.SetSubjectDemographics(fullName, nd.Patient.PatientID, gender, nd.Patient.BirthDate, types.RACE_OTHER)

	sdp := h.HL7AEcg.ComponentOf.TimepointEvent.ComponentOf.SubjectAssignment.Subject.TrialSubject.SubjectDemographicPerson
	if fullName == "" {
		sdp.Name = nil
	}
	if nd.Patient.BirthDate == "" {
		sdp.BirthTime = nil
	}

	// ClinicalTrial
	ct := &h.HL7AEcg.ComponentOf.TimepointEvent.ComponentOf.SubjectAssignment.ComponentOf.ClinicalTrial
	trialExt := baseName
	if startDT != "" {
		trialExt = baseName + "-" + startDT
	}
	ct.SetID(rootUUID, trialExt)

	// Location
	h.SetLocation("trialSite", rootUUID, nd.Patient.Location, "", "", "")
	h.SetResponsibleParty(rootUUID, "trialInvestigator", "", "", "", "")

	// Rhythm series with 12 leads
	if len(nd.Leads) > 0 && startDT != "" {
		leads12 := Build12LeadMap(nd.Leads)
		h.AddRhythmSeries(startDT, endDT, nil, nil, float64(nd.Record.SampleRate), leads12, 0.0, nd.Record.Scale)

		// Series ID
		lastComp := &h.HL7AEcg.Component[len(h.HL7AEcg.Component)-1]
		lastComp.Series.ID = &types.ID{Root: rootUUID, Extension: nd.Patient.PatientID}

		// Device author
		if nd.Patient.DeviceModel != "" {
			model := nd.Patient.DeviceModel
			manufacturer := "Nihon Kohden"
			serial := ""
			software := ""
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

		// Annotations
		addAnnotations(h, nd, startDT)
	}

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

func addAnnotations(h *hl7aecg.Hl7xml, nd *NKData, studyDT string) {
	if len(h.HL7AEcg.Component) == 0 {
		return
	}
	series := &h.HL7AEcg.Component[len(h.HL7AEcg.Component)-1].Series
	annSet := series.InitAnnotationSet(studyDT)

	m := nd.Measurement
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

func recordingDuration(nSamples, sampleRate int) time.Duration {
	if sampleRate == 0 {
		return 0
	}
	return time.Duration(nSamples) * time.Second / time.Duration(sampleRate)
}

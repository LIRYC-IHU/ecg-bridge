package fukudatofda

import (
	"context"
	"crypto/rand"
	stdxml "encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/LIRYC-IHU/ecg-bridge/metaject"

	"github.com/LIRYC-IHU/hl7v3-aecg/hl7aecg"
	"github.com/LIRYC-IHU/hl7v3-aecg/hl7aecg/types"
)

// Convert parses a Fukuda .ECG file and writes FDA aECG XML to outputPath.
// If outputPath is empty, output is written to stdout. When anonymize is true,
// direct patient identifiers are stripped. When meta is non-nil, its fields
// overwrite the parsed metadata. lang selects statement text ("en" or "fr").
func Convert(inputPath, outputPath string, anonymize bool, meta *metaject.Override, lang string) error {
	dat, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", inputPath, err)
	}

	fd, err := ParseFile(dat)
	if err != nil {
		return fmt.Errorf("parsing Fukuda file: %w", err)
	}

	if anonymize {
		fd.Anonymize()
	}
	fd.ApplyMetadata(meta)

	baseName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))

	xmlStr, err := buildAECG(fd, baseName, lang)
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

func buildAECG(fd *FukudaData, baseName, lang string) (string, error) {
	h := hl7aecg.NewHl7xml("")
	h.Initialize(types.CPT_CODE_ECG_Routine, types.CPT_OID, "CPT-4", "")
	h.HL7AEcg.ConfidentialityCode = nil
	h.HL7AEcg.ReasonCode = nil

	rootUUID := newUUID()
	h.HL7AEcg.SetRootID(rootUUID, "")

	dt := fd.Patient.RecordingAt
	var startDT, endDT string
	if !dt.IsZero() {
		startDT = dt.Format("20060102150405")
		dur := recordingDuration(fd.Record.TotalSamples, fd.Record.SampleRate)
		endDT = dt.Add(dur).Format("20060102150405")
		h.SetEffectiveTime(startDT, startDT, nil, nil)
	}

	// Subject and demographics
	h.SetSubject(rootUUID, fd.Patient.PatientID, types.SUBJECT_ROLE_ENROLLED)
	fullName := strings.TrimSpace(fd.Patient.FamilyName + " " + fd.Patient.GivenName)
	gender := types.GetGender(fd.Patient.Gender)
	h.SetSubjectDemographics(fullName, fd.Patient.PatientID, gender, fd.Patient.BirthDate, types.RACE_OTHER)

	sdp := h.HL7AEcg.ComponentOf.TimepointEvent.ComponentOf.SubjectAssignment.Subject.TrialSubject.SubjectDemographicPerson
	if fullName == "" {
		sdp.Name = nil
	}
	if fd.Patient.BirthDate == "" {
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
	h.SetLocation("trialSite", rootUUID, fd.Patient.Location, "", "", "")
	h.SetResponsibleParty(rootUUID, "trialInvestigator", "", "", "", "")

	// Rhythm series with 12 leads
	if len(fd.Leads) > 0 && startDT != "" {
		leads12 := Build12LeadMap(fd.Leads)
		h.AddRhythmSeries(startDT, endDT, nil, nil, float64(fd.Record.SampleRate), leads12, 0.0, fd.Record.Scale)

		lastComp := &h.HL7AEcg.Component[len(h.HL7AEcg.Component)-1]
		lastComp.Series.ID = &types.ID{Root: rootUUID, Extension: fd.Patient.PatientID}

		// Device author
		model := fd.Patient.DeviceModel
		if model == "" {
			model = "FX-ECG"
		}
		manufacturer := "Fukuda Denshi"
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

		addAnnotations(h, fd, startDT, lang)
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

// addAnnotations emits the interpretive ECG statements (resolved to text in the
// requested language) as coded annotations on the rhythm series.
func addAnnotations(h *hl7aecg.Hl7xml, fd *FukudaData, studyDT, lang string) {
	if len(h.HL7AEcg.Component) == 0 {
		return
	}
	series := &h.HL7AEcg.Component[len(h.HL7AEcg.Component)-1].Series
	annSet := series.InitAnnotationSet(studyDT)

	for _, st := range fd.Statements {
		txt := statementText(lang, st.Code)
		if txt == "" {
			continue
		}
		annSet.AddTextAnnotation(st.Code, statementCodeSystem, txt)
	}

	m := fd.Measurement
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
	if m.HasQRSAxis {
		annSet.AddAnnotation(string(types.MDC_ECG_ANGLE_QRS_FRONT), string(types.MDC_OID), float64(m.QRSAxis), "deg")
	}
}

func recordingDuration(nSamples, sampleRate int) time.Duration {
	if sampleRate == 0 {
		return 0
	}
	return time.Duration(nSamples) * time.Second / time.Duration(sampleRate)
}

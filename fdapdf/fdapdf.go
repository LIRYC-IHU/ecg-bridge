// Package fdapdf maps a parsed FDA aECG XML document into a vendor-neutral
// ecgpdf.Report. Because every converter in this repo can emit FDA aECG XML,
// this is the single "via FDA" path to a PDF: a vendor front-end converts to
// FDA, then this package renders it — no per-vendor PDF code required.
//
// Note: the current FDA parser (fda-to-dicom.ParseFDA) does not expose the
// interpretive statement list, so reports produced through this path carry no
// interpretation text yet.
package fdapdf

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/LIRYC-IHU/ecg-bridge/ecgpdf"
	fdatodicom "github.com/LIRYC-IHU/ecg-bridge/fda-to-dicom"
)

// ReportFromFile parses an FDA aECG XML file and maps it to an ecgpdf.Report.
func ReportFromFile(path string) (*ecgpdf.Report, error) {
	d, err := fdatodicom.ParseFDA(path)
	if err != nil {
		return nil, fmt.Errorf("parsing FDA XML: %w", err)
	}
	return FromFDA(d), nil
}

// FromFDA maps an already-parsed FDAData to an ecgpdf.Report. FDA aECG already
// stores all 12 leads by name, so no augmented-lead derivation is needed.
func FromFDA(d *fdatodicom.FDAData) *ecgpdf.Report {
	leadMap := make(map[string][]int32, len(d.Leads))
	for name, src := range d.Leads {
		dst := make([]int32, len(src))
		for j, v := range src {
			dst[j] = int32(v)
		}
		leadMap[name] = dst
	}

	model := d.ModelName
	if strings.TrimSpace(model) == "" {
		model = d.Manufacturer
	}

	var sts []ecgpdf.Statement
	if s := strings.TrimSpace(d.InterpretationSummary); s != "" {
		sts = append(sts, ecgpdf.Statement{Text: s, Emphasis: true})
	}
	for _, st := range d.InterpretationStatements {
		if st = strings.TrimSpace(st); st != "" {
			sts = append(sts, ecgpdf.Statement{Text: st})
		}
	}
	if c := strings.TrimSpace(d.InterpretationComment); c != "" {
		sts = append(sts, ecgpdf.Statement{Text: c})
	}

	return &ecgpdf.Report{
		PatientID:   d.PatientID,
		Name:        strings.TrimSpace(strings.ReplaceAll(d.PatientName, "^", " ")),
		Sex:         d.PatientSex,
		BirthDate:   d.PatientDOB,
		Age:         normalizeAge(d.PatientAge),
		DeviceModel: model,
		Department:  d.InstitutionName,
		Operator:    d.OperatorID,
		RecordingAt: studyTime(d.StudyDate, d.StudyTime),
		HeartRate:   round(d.HeartRate),
		PRInterval:  round(d.PRInterval),
		QRSDuration: round(d.QRSDuration),
		QTInterval:  round(d.QTInterval),
		QTcInterval: round(d.QTcInterval),
		PAxis:       round(d.PFrontAxis),
		QRSAxis:     round(d.QRSFrontAxis),
		TAxis:       round(d.TFrontAxis),
		Filter:      filterSpec(d.FilterHPF, d.FilterLPF),
		SampleRate:  d.SamplingRate,
		ScaleUV:     d.Sensitivity,
		Leads:       leadMap,
		Statements:  sts,
	}
}

// normalizeAge turns a DICOM age string like "050Y" into "50".
func normalizeAge(a string) string {
	a = strings.TrimSpace(a)
	a = strings.TrimRight(a, "YyMmWwDd")
	return strings.TrimLeft(a, "0")
}

// studyTime combines a YYYYMMDD date and HHMMSS time into a UTC time.
func studyTime(date, t string) time.Time {
	if len(date) != 8 {
		return time.Time{}
	}
	if len(t) != 6 {
		t = "000000"
	}
	tm, err := time.ParseInLocation("20060102150405", date+t, time.UTC)
	if err != nil {
		return time.Time{}
	}
	return tm
}

// filterSpec formats the acquisition band-pass, or "" when unknown.
func filterSpec(hpf, lpf float64) string {
	switch {
	case hpf > 0 && lpf > 0:
		return fmt.Sprintf("%g–%g Hz", hpf, lpf)
	case lpf > 0:
		return fmt.Sprintf("≤%g Hz", lpf)
	default:
		return ""
	}
}

func round(v float64) int { return int(math.Round(v)) }

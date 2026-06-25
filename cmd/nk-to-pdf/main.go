// Command nk-to-pdf converts a Nihon Kohden .DAT (PEC) recording into a
// printable 12-lead ECG PDF report (NK paper style). Every metadata field is
// real selectable text and the waveforms are vector polylines on a millimetric
// grid, so the output zooms cleanly. Rendering is shared with the other vendor
// PDF tools via the converter-fda/ecgpdf package.
//
//	nk-to-pdf -i input.DAT -o out.pdf          # write a file
//	nk-to-pdf -i input.DAT | base64 -d > x.pdf # base64 on stdout
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"

	"converter-fda/ecgpdf"
	nktofda "converter-fda/nk-to-fda"
)

func main() {
	var in, out, lang string
	flag.StringVar(&in, "i", "", "input NK .DAT file (required)")
	flag.StringVar(&out, "o", "", "output PDF path; if omitted, prints the base64-encoded PDF to stdout")
	flag.StringVar(&lang, "l", "en", "interpretive statement language: en or fr")
	flag.Parse()

	if in == "" {
		fmt.Fprintln(os.Stderr, "error: -i input file is required")
		flag.Usage()
		os.Exit(2)
	}

	dat, err := os.ReadFile(in)
	if err != nil {
		fail("reading input: %v", err)
	}
	nd, err := nktofda.ParseFile(dat)
	if err != nil {
		fail("parsing NK file: %v", err)
	}
	leads, err := nktofda.DecodeWaveforms(dat, nd.Record.TotalSamples)
	if err != nil {
		fail("decoding waveforms: %v", err)
	}
	nd.Leads = leads

	rep := buildReport(nd, lang)

	var buf bytes.Buffer
	if err := ecgpdf.Render(rep, lang, &buf); err != nil {
		fail("rendering PDF: %v", err)
	}
	if err := ecgpdf.Output(buf.Bytes(), out); err != nil {
		fail("writing output: %v", err)
	}
	if out != "" {
		fmt.Fprintf(os.Stderr, "Wrote %s\n", out)
	}
}

// buildReport maps NK-specific data into the vendor-neutral ecgpdf.Report,
// deriving the 4 augmented leads and resolving statement codes to text.
func buildReport(nd *nktofda.NKData, lang string) *ecgpdf.Report {
	p, m := nd.Patient, nd.Measurement

	i, ii := nd.Leads["I"], nd.Leads["II"]
	iii, avr, avl, avf := nktofda.DeriveLeads(i, ii)
	leadMap := map[string][]int32{
		"I": i, "II": ii, "III": iii, "aVR": avr, "aVL": avl, "aVF": avf,
		"V1": nd.Leads["V1"], "V2": nd.Leads["V2"], "V3": nd.Leads["V3"],
		"V4": nd.Leads["V4"], "V5": nd.Leads["V5"], "V6": nd.Leads["V6"],
	}

	var sts []ecgpdf.Statement
	for _, s := range nd.Statements {
		txt := nktofda.StatementText(lang, s.Code)
		if txt == "" {
			continue
		}
		sts = append(sts, ecgpdf.Statement{Code: s.Code, Text: txt, Emphasis: s.Overall})
	}

	return &ecgpdf.Report{
		PatientID:      p.PatientID,
		Name:           strings.TrimSpace(p.FamilyName + " " + p.GivenName),
		Sex:            p.Gender,
		BirthDate:      p.BirthDate,
		Age:            p.Age,
		Height:         p.Height,
		Weight:         p.Weight,
		BloodPressure:  strings.Join(strings.Fields(p.BloodPressure), "/"),
		Medications:    p.Medications,
		History:        p.History,
		Symptoms:       p.Symptoms,
		DeviceModel:    p.DeviceModel,
		Department:     p.Department,
		Operator:       p.Operator,
		Location:       p.Location,
		RecordingAt:    p.RecordingAt,
		HeartRate:      m.HeartRate,
		PRInterval:     m.PRInterval,
		QRSDuration:    m.QRSDuration,
		QTInterval:     m.QTInterval,
		QTcInterval:    m.QTcInterval,
		PAxis:          m.PAxis,
		QRSAxis:        m.QRSAxis,
		TAxis:          m.TAxis,
		ShowAmplitudes: true,
		V5RAmplitude:   m.V5RAmplitude,
		V1SAmplitude:   m.V1SAmplitude,
		Filter:         "H50–150 Hz",
		SampleRate:     float64(nd.Record.SampleRate),
		ScaleUV:        nd.Record.Scale,
		Leads:          leadMap,
		Statements:     sts,
	}
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "nk-to-pdf: "+format+"\n", a...)
	os.Exit(1)
}

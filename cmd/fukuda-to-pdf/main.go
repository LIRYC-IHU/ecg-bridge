// Command fukuda-to-pdf converts a Fukuda Denshi .ECG recording into a
// printable 12-lead ECG PDF report. Every metadata field is real selectable
// text and the waveforms are vector polylines on a millimetric grid, so the
// output zooms cleanly. Rendering is shared with the other vendor PDF tools via
// the converter-fda/ecgpdf package.
//
//	fukuda-to-pdf -i input.ECG -o out.pdf          # write a file
//	fukuda-to-pdf -i input.ECG | base64 -d > x.pdf # base64 on stdout
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/LIRYC-IHU/ecg-bridge/ecgpdf"
	fukudatofda "github.com/LIRYC-IHU/ecg-bridge/fukuda-to-fda"
)

// Version reports the build version.
func Version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	if info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}

func main() {
	var in, out, lang string
	flag.StringVar(&in, "i", "", "input Fukuda .ECG file (required)")
	flag.StringVar(&out, "o", "", "output PDF path; if omitted, prints the base64-encoded PDF to stdout")
	flag.StringVar(&lang, "l", "en", "interpretive statement language: en or fr")
	var identityUnverified bool
	flag.BoolVar(&identityUnverified, "identity-unverified", false,
		"mark the document's identity as recorded by the acquisition device and not confirmed against the hospital information system")
	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()
	if showVersion {
		fmt.Println(Version())
		return
	}

	if in == "" {
		fmt.Fprintln(os.Stderr, "error: -i input file is required")
		flag.Usage()
		os.Exit(2)
	}

	dat, err := os.ReadFile(in)
	if err != nil {
		fail("reading input: %v", err)
	}
	fd, err := fukudatofda.ParseFile(dat)
	if err != nil {
		fail("parsing Fukuda file: %v", err)
	}

	rep := buildReport(fd, lang)

	rep.IdentityUnverified = identityUnverified

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

// buildReport maps Fukuda data into the vendor-neutral ecgpdf.Report, deriving
// the 4 augmented leads and resolving statement codes to text.
func buildReport(fd *fukudatofda.FukudaData, lang string) *ecgpdf.Report {
	p, m := fd.Patient, fd.Measurement

	i, ii := fd.Leads["I"], fd.Leads["II"]
	iii, avr, avl, avf := fukudatofda.DeriveLeads(i, ii)
	leadMap := map[string][]int32{
		"I": i, "II": ii, "III": iii, "aVR": avr, "aVL": avl, "aVF": avf,
		"V1": fd.Leads["V1"], "V2": fd.Leads["V2"], "V3": fd.Leads["V3"],
		"V4": fd.Leads["V4"], "V5": fd.Leads["V5"], "V6": fd.Leads["V6"],
	}

	var sts []ecgpdf.Statement
	for _, s := range fd.Statements {
		txt := fukudatofda.StatementText(lang, s.Code)
		if txt == "" {
			continue
		}
		sts = append(sts, ecgpdf.Statement{Code: s.Code, Text: txt})
	}

	return &ecgpdf.Report{
		PatientID:   p.PatientID,
		Name:        strings.TrimSpace(p.FamilyName + " " + p.GivenName),
		Sex:         p.Gender,
		BirthDate:   p.BirthDate,
		DeviceModel: p.DeviceModel,
		Location:    p.Location,
		RecordingAt: p.RecordingAt,
		HeartRate:   ecgpdf.MeasuredNonZero(m.HeartRate),
		PRInterval:  ecgpdf.MeasuredNonZero(m.PRInterval),
		QRSDuration: ecgpdf.MeasuredNonZero(m.QRSDuration),
		QTInterval:  ecgpdf.MeasuredNonZero(m.QTInterval),
		QTcInterval: ecgpdf.MeasuredNonZero(m.QTcInterval),
		QRSAxis:     ecgpdf.MeasuredNonZero(m.QRSAxis),
		SampleRate:  float64(fd.Record.SampleRate),
		ScaleUV:     fd.Record.Scale,
		Leads:       leadMap,
		Statements:  sts,
	}
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "fukuda-to-pdf: "+format+"\n", a...)
	os.Exit(1)
}

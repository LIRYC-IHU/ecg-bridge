// Command philips-to-pdf renders a Philips SierraECG XML file into a 12-lead
// ECG PDF. It is a thin wrapper over the universal "via FDA" path: it converts
// Philips → FDA aECG XML, then renders that FDA document with the shared
// renderer (same as fda-to-pdf). Every non-NK vendor follows this pattern.
//
//	philips-to-pdf -i ecg.xml -o out.pdf
//	philips-to-pdf -i ecg.xml | base64 -d > out.pdf
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"converter-fda/ecgpdf"
	"converter-fda/fdapdf"
	philipstofda "converter-fda/philips-to-fda"
)

func main() {
	var in, out, lang string
	var forms bool
	flag.StringVar(&in, "i", "", "input Philips SierraECG XML file (required)")
	flag.StringVar(&out, "o", "", "output PDF path; if omitted, prints the base64-encoded PDF to stdout")
	flag.StringVar(&lang, "l", "en", "report language: en or fr")
	flag.BoolVar(&forms, "forms", true, "render patient/measurement values as fillable AcroForm fields")
	flag.Parse()

	if in == "" {
		fmt.Fprintln(os.Stderr, "error: -i input file is required")
		flag.Usage()
		os.Exit(2)
	}

	// Step 1: Philips → FDA aECG XML (via a temp file).
	tmp, err := os.CreateTemp("", "philips-*.fda.xml")
	if err != nil {
		fail("creating temp file: %v", err)
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	if err := philipstofda.Convert(in, tmp.Name(), false, nil); err != nil {
		fail("converting Philips → FDA: %v", err)
	}

	// Step 2: FDA → PDF (shared renderer).
	rep, err := fdapdf.ReportFromFile(tmp.Name())
	if err != nil {
		fail("%v", err)
	}
	rep.Forms = forms

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

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "philips-to-pdf: "+format+"\n", a...)
	os.Exit(1)
}

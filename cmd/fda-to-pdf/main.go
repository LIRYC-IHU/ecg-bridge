// Command fda-to-pdf renders an FDA aECG XML file into a printable 12-lead ECG
// PDF report. Since every converter in this repo can emit FDA aECG XML, this is
// the universal PDF path: `<vendor>-to-fda | fda-to-pdf`.
//
//	fda-to-pdf -i ecg_fda.xml -o out.pdf
//	fda-to-pdf -i ecg_fda.xml | base64 -d > out.pdf
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"converter-fda/ecgpdf"
	"converter-fda/fdapdf"
)

func main() {
	var in, out, lang string
	var forms bool
	flag.StringVar(&in, "i", "", "input FDA aECG XML file (required)")
	flag.StringVar(&out, "o", "", "output PDF path; if omitted, prints the base64-encoded PDF to stdout")
	flag.StringVar(&lang, "l", "en", "report language: en or fr")
	flag.BoolVar(&forms, "forms", true, "render patient/measurement values as fillable AcroForm fields")
	flag.Parse()

	if in == "" {
		fmt.Fprintln(os.Stderr, "error: -i input file is required")
		flag.Usage()
		os.Exit(2)
	}

	rep, err := fdapdf.ReportFromFile(in)
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
	fmt.Fprintf(os.Stderr, "fda-to-pdf: "+format+"\n", a...)
	os.Exit(1)
}

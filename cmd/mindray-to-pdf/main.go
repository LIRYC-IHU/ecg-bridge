// Command mindray-to-pdf renders a Mindray BeneHeart R12 .PAT file into a
// 12-lead ECG PDF. It is a thin wrapper over the universal "via FDA" path:
// Mindray → FDA aECG XML → shared renderer (same as fda-to-pdf).
//
//	mindray-to-pdf -i ecg.PAT -o out.pdf
//	mindray-to-pdf -i ecg.PAT | base64 -d > out.pdf
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"converter-fda/ecgpdf"
	"converter-fda/fdapdf"
	mindraytofda "converter-fda/mindray-to-fda"
)

func main() {
	var in, out, lang string
	flag.StringVar(&in, "i", "", "input Mindray .PAT file (required)")
	flag.StringVar(&out, "o", "", "output PDF path; if omitted, prints the base64-encoded PDF to stdout")
	flag.StringVar(&lang, "l", "en", "report language: en or fr")
	flag.Parse()

	if in == "" {
		fmt.Fprintln(os.Stderr, "error: -i input file is required")
		flag.Usage()
		os.Exit(2)
	}

	// Step 1: Mindray → FDA aECG XML (via a temp file).
	tmp, err := os.CreateTemp("", "mindray-*.fda.xml")
	if err != nil {
		fail("creating temp file: %v", err)
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	if err := mindraytofda.Convert(in, tmp.Name(), false, nil); err != nil {
		fail("converting Mindray → FDA: %v", err)
	}

	// Step 2: FDA → PDF (shared renderer).
	rep, err := fdapdf.ReportFromFile(tmp.Name())
	if err != nil {
		fail("%v", err)
	}

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
	fmt.Fprintf(os.Stderr, "mindray-to-pdf: "+format+"\n", a...)
	os.Exit(1)
}

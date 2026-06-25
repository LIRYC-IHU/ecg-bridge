// Command muse-to-pdf renders a GE MUSE XML file into a 12-lead ECG PDF. It is
// a thin wrapper over the universal "via FDA" path: MUSE → FDA aECG XML →
// shared renderer (same as fda-to-pdf).
//
//	muse-to-pdf -i ecg.xml -o out.pdf
//	muse-to-pdf -i ecg.xml | base64 -d > out.pdf
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"converter-fda/ecgpdf"
	"converter-fda/fdapdf"
	musetofda "converter-fda/muse-to-fda"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	var in, out, lang string
	flag.StringVar(&in, "i", "", "input GE MUSE XML file (required)")
	flag.StringVar(&out, "o", "", "output PDF path; if omitted, prints the base64-encoded PDF to stdout")
	flag.StringVar(&lang, "l", "en", "report language: en or fr")
	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()
	if showVersion {
		fmt.Println(version)
		return
	}

	if in == "" {
		fmt.Fprintln(os.Stderr, "error: -i input file is required")
		flag.Usage()
		os.Exit(2)
	}

	// Step 1: MUSE → FDA aECG XML (via a temp file).
	tmp, err := os.CreateTemp("", "muse-*.fda.xml")
	if err != nil {
		fail("creating temp file: %v", err)
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	if err := musetofda.Convert(in, tmp.Name(), false, nil); err != nil {
		fail("converting MUSE → FDA: %v", err)
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
	fmt.Fprintf(os.Stderr, "muse-to-pdf: "+format+"\n", a...)
	os.Exit(1)
}

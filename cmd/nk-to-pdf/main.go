// Command nk-to-pdf is a PROTOTYPE: it converts a Nihon Kohden .DAT (PEC)
// recording into a printable 12-lead ECG PDF report that mimics the NK paper
// output. Unlike a scanned image, every metadata field is real selectable text
// and the waveforms are vector polylines on a millimetric (red graph-paper)
// grid.
//
// This lives under proto/ on purpose — it is a throwaway/experimental tool we
// will refine once the basic PDF output looks right.
//
//	go run ./proto/nk-to-pdf -i input.DAT -o out.pdf
package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"os"

	nktofda "converter-fda/nk-to-fda"
)

func main() {
	var (
		in   string
		out  string
		lang string
	)
	flag.StringVar(&in, "i", "", "input NK .DAT file (required)")
	flag.StringVar(&out, "o", "", "output PDF path; if omitted, prints the base64-encoded PDF to stdout")
	flag.StringVar(&lang, "l", "fr", "interpretive statement language: en or fr")
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

	var buf bytes.Buffer
	if err := renderPDF(nd, lang, &buf); err != nil {
		fail("rendering PDF: %v", err)
	}

	if out == "" {
		// No -o: emit the raw PDF as base64 on stdout so external tools can
		// recover the original bytes (e.g. `... | base64 -d > ecg.pdf`).
		enc := base64.NewEncoder(base64.StdEncoding, os.Stdout)
		if _, err := enc.Write(buf.Bytes()); err != nil {
			fail("writing base64 output: %v", err)
		}
		if err := enc.Close(); err != nil {
			fail("flushing base64 output: %v", err)
		}
		fmt.Fprintln(os.Stdout)
		return
	}

	if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
		fail("writing %s: %v", out, err)
	}
	fmt.Fprintf(os.Stderr, "Wrote %s\n", out)
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "nk-to-pdf: "+format+"\n", a...)
	os.Exit(1)
}

package mindraytodicom

import (
	"fmt"
	"os"

	dicomconf "converter-fda/dicomconf"
	mindraytofda "converter-fda/mindray-to-fda"

	"github.com/suyashkumar/dicom"
)

// Convert parses a Mindray file and writes a DICOM ECG file.
// When anonymize is true, direct patient identifiers are stripped from the output.
func Convert(inputPath, outputPath string, anonymize bool) error {
	dat, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", inputPath, err)
	}

	md, err := mindraytofda.ParseFile(dat)
	if err != nil {
		return fmt.Errorf("parsing Mindray file: %w", err)
	}

	if anonymize {
		md.Anonymize()
	}

	ds, err := BuildDICOM(md)
	if err != nil {
		return fmt.Errorf("building DICOM: %w", err)
	}

	var f *os.File
	if outputPath == "" {
		f = os.Stdout
	} else {
		f, err = os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
	}
	defer f.Close()

	dicomconf.Finalize(&ds)
	if err := dicom.Write(f, ds); err != nil {
		return fmt.Errorf("writing DICOM: %w", err)
	}
	return nil
}

package mindraytodicom

import (
	"fmt"
	"os"

	dicomconf "github.com/LIRYC-IHU/ecg-bridge/dicomconf"
	"github.com/LIRYC-IHU/ecg-bridge/metaject"
	mindraytofda "github.com/LIRYC-IHU/ecg-bridge/mindray-to-fda"

	"github.com/suyashkumar/dicom"
)

// Convert parses a Mindray file and writes a DICOM ECG file.
// When anonymize is true, direct patient identifiers are stripped from the output.
// When meta is non-nil, its fields overwrite the parsed metadata (injection).
func Convert(inputPath, outputPath string, anonymize bool, meta *metaject.Override) error {
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
	md.ApplyMetadata(meta)

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

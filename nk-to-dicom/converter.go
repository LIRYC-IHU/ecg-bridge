package nktodicom

import (
	"fmt"
	"os"

	dicomconf "converter-fda/dicomconf"
	nktofda "converter-fda/nk-to-fda"

	"github.com/suyashkumar/dicom"
)

// Convert reads a NK .DAT file and writes a DICOM ECG file.
func Convert(inputPath, outputPath string) error {
	// 1. Read and parse NK file
	dat, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", inputPath, err)
	}

	nd, err := nktofda.ParseFile(dat)
	if err != nil {
		return fmt.Errorf("parsing NK file: %w", err)
	}

	// 2. Decode waveforms (all 8 measured leads, incl. QRS-zone reconstruction).
	leads, err := nktofda.DecodeWaveforms(dat, nd.Record.TotalSamples)
	if err != nil {
		return fmt.Errorf("decoding waveforms: %w", err)
	}
	nd.Leads = leads

	// 3. Build DICOM dataset
	ds, err := BuildDICOM(nd)
	if err != nil {
		return fmt.Errorf("building DICOM: %w", err)
	}

	dicomconf.Finalize(ds)

	// 4. Write DICOM file

	if outputPath != "" {
		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer f.Close()
		if err := dicom.Write(f, *ds); err != nil {
			return fmt.Errorf("writing DICOM: %w", err)
		}
	} else {
		if err := dicom.Write(os.Stdout, *ds); err != nil {
			return fmt.Errorf("writing DICOM: %w", err)
		}
	}

	return nil
}

package philipstodicom

import (
	"fmt"
	"os"

	"github.com/suyashkumar/dicom"
)

// Convert reads a Philips SierraECG XML file and writes a DICOM ECG file.
func Convert(inputPath, outputPath string) error {
	data, err := ParsePhilips(inputPath)
	if err != nil {
		return fmt.Errorf("parsing Philips XML: %w", err)
	}

	ds, err := BuildDICOM(data)
	if err != nil {
		return fmt.Errorf("building DICOM: %w", err)
	}

	var f *os.File
	if outputPath == "" {
		// Write to stdout
		f = os.Stdout
	} else {
		f, err = os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
	}
	defer f.Close()

	if err := dicom.Write(f, ds); err != nil {
		return fmt.Errorf("writing DICOM: %w", err)
	}
	return nil
}

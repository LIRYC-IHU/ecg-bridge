package mindraytodicom

import (
	"fmt"
	"os"

	mindraytofda "converter-fda/mindray-to-fda"

	"github.com/suyashkumar/dicom"
)

func Convert(inputPath, outputPath string) error {
	dat, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", inputPath, err)
	}

	md, err := mindraytofda.ParseFile(dat)
	if err != nil {
		return fmt.Errorf("parsing Mindray file: %w", err)
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

	if err := dicom.Write(f, ds); err != nil {
		return fmt.Errorf("writing DICOM: %w", err)
	}
	return nil
}

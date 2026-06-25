package philipstodicom

import (
	"fmt"
	"os"

	dicomconf "github.com/LIRYC-IHU/ecg-bridge/dicomconf"
	"github.com/LIRYC-IHU/ecg-bridge/metaject"

	"github.com/suyashkumar/dicom"
)

// Options holds optional conversion parameters.
type Options struct {
	Anonymize bool
	Meta      *metaject.Override // non-nil → inject/overwrite parsed metadata
}

// Convert reads a Philips SierraECG XML file and writes a DICOM ECG file.
func Convert(inputPath, outputPath string) error {
	return ConvertWithOptions(inputPath, outputPath, Options{})
}

// ConvertWithOptions is like Convert but accepts additional options.
func ConvertWithOptions(inputPath, outputPath string, opts Options) error {
	data, err := ParsePhilips(inputPath)
	if err != nil {
		return fmt.Errorf("parsing Philips XML: %w", err)
	}

	if opts.Anonymize {
		data.Anonymize()
	}
	data.ApplyMetadata(opts.Meta)

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

	dicomconf.Finalize(&ds)
	if err := dicom.Write(f, ds); err != nil {
		return fmt.Errorf("writing DICOM: %w", err)
	}
	return nil
}

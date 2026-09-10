package fukudatodicom

import (
	"fmt"
	"os"

	dicomconf "github.com/LIRYC-IHU/ecg-bridge/dicomconf"
	fukudatofda "github.com/LIRYC-IHU/ecg-bridge/fukuda-to-fda"
	"github.com/LIRYC-IHU/ecg-bridge/metaject"

	"github.com/suyashkumar/dicom"
)

// Convert reads a Fukuda .ECG file and writes a 12-lead ECG DICOM file.
// When anonymize is true, direct patient identifiers are stripped from the output.
// When meta is non-nil, its fields overwrite the parsed metadata (injection).
func Convert(inputPath, outputPath string, anonymize bool, meta *metaject.Override) error {
	dat, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", inputPath, err)
	}

	fd, err := fukudatofda.ParseFile(dat)
	if err != nil {
		return fmt.Errorf("parsing Fukuda file: %w", err)
	}

	if anonymize {
		fd.Anonymize()
	}
	fd.ApplyMetadata(meta)

	ds, err := BuildDICOM(fd)
	if err != nil {
		return fmt.Errorf("building DICOM: %w", err)
	}

	dicomconf.Finalize(ds)

	if outputPath != "" {
		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer f.Close()
		if err := dicom.Write(f, *ds); err != nil {
			return fmt.Errorf("writing DICOM: %w", err)
		}
		return nil
	}
	if err := dicom.Write(os.Stdout, *ds); err != nil {
		return fmt.Errorf("writing DICOM: %w", err)
	}
	return nil
}

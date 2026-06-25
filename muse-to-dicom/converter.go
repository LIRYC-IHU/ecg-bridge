package musetodicom

import (
	"fmt"
	"os"

	dicomconf "github.com/LIRYC-IHU/ecg-bridge/dicomconf"
	"github.com/LIRYC-IHU/ecg-bridge/metaject"
	musetofda "github.com/LIRYC-IHU/ecg-bridge/muse-to-fda"

	"github.com/suyashkumar/dicom"
)

// Convert parses a GE MUSE RestingECG XML file and writes a DICOM
// 12-lead ECG waveform file. If outputPath is empty, output goes to stdout.
// When anonymize is true, direct patient identifiers are stripped from the output.
// When meta is non-nil, its fields overwrite the parsed metadata (injection).
func Convert(inputPath, outputPath string, anonymize bool, meta *metaject.Override) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", inputPath, err)
	}

	d, err := musetofda.ParseMuse(data)
	if err != nil {
		return fmt.Errorf("parsing MUSE XML: %w", err)
	}

	if anonymize {
		d.Anonymize()
	}
	d.ApplyMetadata(meta)

	ds, err := BuildDICOM(d)
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

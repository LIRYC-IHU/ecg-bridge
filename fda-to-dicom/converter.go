package fdatodicom

import (
	"fmt"
	"os"
	"strings"

	dicomconf "github.com/LIRYC-IHU/ecg-bridge/dicomconf"
	"github.com/LIRYC-IHU/ecg-bridge/metaject"

	"github.com/suyashkumar/dicom"
)

// Convert reads an FDA aECG XML file and writes a DICOM ECG file.
// If outputPath is empty, the DICOM bytes are written to stdout (matching the
// other *-to-dicom converters so the ECGBridge can read the result from stdout).
// When anonymize is true, direct patient identifiers are stripped from the output.
// When meta is non-nil, its fields overwrite the parsed metadata (injection).
func Convert(inputPath, outputPath string, anonymize bool, meta *metaject.Override) error {
	data, err := ParseFDA(inputPath)
	if err != nil {
		return fmt.Errorf("parsing FDA XML: %w", err)
	}

	if anonymize {
		data.Anonymize()
	}
	data.ApplyMetadata(meta)

	ds, err := BuildDICOM(data)
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

// ValidateFDAInput performs basic content validation of the input file.
func ValidateFDAInput(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	head := string(raw)
	if len(head) > 4096 {
		head = head[:4096]
	}
	if !strings.Contains(head, "AnnotatedECG") || !strings.Contains(head, "urn:hl7-org:v3") {
		return fmt.Errorf("not an FDA aECG XML file (AnnotatedECG element not found)")
	}
	return nil
}

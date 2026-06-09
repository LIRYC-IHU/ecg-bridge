package fdatodicom

import (
	"fmt"
	"os"
	"strings"

	dicomconf "converter-fda/dicomconf"

	"github.com/suyashkumar/dicom"
)

// Convert reads an FDA aECG XML file and writes a DICOM ECG file.
func Convert(inputPath, outputPath string) error {
	data, err := ParseFDA(inputPath)
	if err != nil {
		return fmt.Errorf("parsing FDA XML: %w", err)
	}

	ds, err := BuildDICOM(data)
	if err != nil {
		return fmt.Errorf("building DICOM: %w", err)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
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

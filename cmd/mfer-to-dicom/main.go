package main

import (
	"fmt"
	"os"

	"github.com/LIRYC-IHU/ecg-bridge/metaject"
	mfertodicom "github.com/LIRYC-IHU/ecg-bridge/mfer-to-dicom"

	"github.com/spf13/cobra"
)

var (
	inputPath  string
	outputPath string
	anonymize  bool
)

var rootCmd = &cobra.Command{
	Use:   "mfer-to-dicom",
	Short: "Convert MFER (.mwf) files to DICOM 12-lead ECG",
	Long: `mfer-to-dicom converts MFER (Medical waveform Format Encoding Rules, .mwf)
ECG files into DICOM 12-lead ECG Waveform Storage (SOP 1.2.840.10008.5.1.4.1.1.9.1.1).

Examples:
  mfer-to-dicom --input ecg.mwf --output ecg.dcm
  mfer-to-dicom --input ecg.mwf > ecg.dcm

Inject metadata (JSON on stdin):
  echo '{"patientID":"123","patientName":"DOE^John"}' | mfer-to-dicom -i ecg.mwf -o out.dcm`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to input MFER .mwf file (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output DICOM file (default: stdout)")
	rootCmd.Flags().BoolVarP(&anonymize, "anonymize", "a", false, "Strip patient-identifying fields from the output")
	_ = rootCmd.MarkFlagRequired("input")
}

func runConvert(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", inputPath)
	}

	meta, err := metaject.FromStdin()
	if err != nil {
		return fmt.Errorf("reading injection metadata from stdin: %w", err)
	}

	if err := mfertodicom.Convert(inputPath, outputPath, anonymize, meta); err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	if outputPath != "" {
		fmt.Fprintf(os.Stderr, "Done. Output written to %s\n", outputPath)
	}
	return nil
}

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

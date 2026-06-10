package main

import (
	"fmt"
	"os"

	nktodicom "converter-fda/nk-to-dicom"

	"github.com/spf13/cobra"
)

var (
	inputPath  string
	outputPath string
	debugMode  bool
	anonymize  bool
)

var rootCmd = &cobra.Command{
	Use:   "nk-to-dicom",
	Short: "Convert Nihon Kohden .DAT files to DICOM ECG format",
	Long: `nk-to-dicom converts Nihon Kohden proprietary ECG files (.DAT / PEC format)
into DICOM 12-lead ECG Waveform Storage format (SOP Class 1.2.840.10008.5.1.4.1.1.9.1.1).

Examples:
  nk-to-dicom --input 00000005.DAT --output ecg.dcm
  nk-to-dicom -i patient.DAT -o output.dcm --debug`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to input NK .DAT file (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output DICOM file (required)")
	rootCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "Print debug information to stderr")
	rootCmd.Flags().BoolVarP(&anonymize, "anonymize", "a", false, "Strip patient-identifying fields (name, ID, birth date) from the output")

	_ = rootCmd.MarkFlagRequired("input")
	_ = rootCmd.MarkFlagRequired("output")
}

func runConvert(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", inputPath)
	}

	fmt.Fprintf(os.Stderr, "Converting %s → %s\n", inputPath, outputPath)

	if err := nktodicom.Convert(inputPath, outputPath, anonymize); err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Done. DICOM file written to %s\n", outputPath)
	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

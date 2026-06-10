package main

import (
	"fmt"
	"os"

	mindraytodicom "converter-fda/mindray-to-dicom"

	"github.com/spf13/cobra"
)

var (
	inputPath  string
	outputPath string
	anonymize  bool
)

var rootCmd = &cobra.Command{
	Use:   "mindray-to-dicom",
	Short: "Convert Mindray BeneHeart R12 files to DICOM ECG format",
	Long: `mindray-to-dicom converts Mindray BeneHeart R12 proprietary ECG files
into DICOM 12-lead ECG Waveform Storage format.

Examples:
  mindray-to-dicom --input 12lead_data_v1 --output ecg.dcm`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to input Mindray file (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output DICOM file (optional, defaults to stdout)")
	rootCmd.Flags().BoolVarP(&anonymize, "anonymize", "a", false, "Strip patient-identifying fields (name, ID, birth date) from the output")

	_ = rootCmd.MarkFlagRequired("input")
}

func runConvert(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", inputPath)
	}

	fmt.Fprintf(os.Stderr, "Converting %s → %s\n", inputPath, outputPath)

	if err := mindraytodicom.Convert(inputPath, outputPath, anonymize); err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Done. Output written to %s\n", outputPath)
	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

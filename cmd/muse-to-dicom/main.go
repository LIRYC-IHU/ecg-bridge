package main

import (
	"fmt"
	"os"

	musetodicom "converter-fda/muse-to-dicom"

	"github.com/spf13/cobra"
)

var (
	inputPath  string
	outputPath string
	anonymize  bool
)

var rootCmd = &cobra.Command{
	Use:   "muse-to-dicom",
	Short: "Convert GE MUSE RestingECG XML files to DICOM ECG format",
	Long: `muse-to-dicom converts GE MUSE RestingECG XML files into
DICOM 12-lead ECG Waveform Storage format.

Examples:
  muse-to-dicom --input ecg.xml --output ecg.dcm`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to input MUSE RestingECG XML file (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output DICOM file (optional, defaults to stdout)")
	rootCmd.Flags().BoolVarP(&anonymize, "anonymize", "a", false, "Strip patient-identifying fields (name, ID, birth date) from the output")

	_ = rootCmd.MarkFlagRequired("input")
}

func runConvert(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", inputPath)
	}

	dest := outputPath
	if dest == "" {
		dest = "stdout"
	}
	fmt.Fprintf(os.Stderr, "Converting %s → %s\n", inputPath, dest)

	if err := musetodicom.Convert(inputPath, outputPath, anonymize); err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	if outputPath != "" {
		fmt.Fprintf(os.Stderr, "Done. Output written to %s\n", outputPath)
	}
	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

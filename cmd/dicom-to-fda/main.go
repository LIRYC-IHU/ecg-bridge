package main

import (
	"fmt"
	"os"
	"strings"

	dicomtofda "github.com/LIRYC-IHU/ecg-bridge/dicom-to-fda"

	"github.com/spf13/cobra"
)

var (
	inputPath    string
	outputPath   string
	metadataPath string
	debugMode    bool
)

var rootCmd = &cobra.Command{
	Use:   "dicom-to-fda",
	Short: "Convert DICOM ECG Waveform files to FDA aECG XML format",
	Long: `dicom-to-fda converts DICOM 12-Lead ECG Waveform Storage (.dcm) files
into FDA-compliant annotated ECG XML (aECG) format.

Examples:
  dicom-to-fda --input ecg.dcm --output ecg_fda.xml
  dicom-to-fda --input ecg.dcm --debug
  dicom-to-fda --input ecg.dcm | xmllint --format -

Inject metadata (JSON file):
  Pass a JSON file with patient/study overrides. A non-empty field overwrites
  the value parsed from the DICOM; an absent or empty field keeps it.
  dicom-to-fda -i ecg.dcm -o out.xml --metadata patient.json`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to input DICOM ECG file (.dcm) (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output FDA XML file (default: stdout)")
	rootCmd.Flags().StringVarP(&metadataPath, "metadata", "m", "", "Path to optional patient metadata JSON file")
	rootCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "Print parsed DICOM data to stderr before converting")

	_ = rootCmd.MarkFlagRequired("input")
}

func runConvert(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", inputPath)
	}
	if outputPath != "" && !strings.HasSuffix(strings.ToLower(outputPath), ".xml") {
		return fmt.Errorf("output must be an FDA aECG XML file (.xml), got: %s", outputPath)
	}

	dest := outputPath
	if dest == "" {
		dest = "stdout"
	}
	fmt.Fprintf(os.Stderr, "Converting %s → %s\n", inputPath, dest)

	if debugMode {
		data, err := dicomtofda.ParseDicom(inputPath)
		if err != nil {
			return fmt.Errorf("parsing DICOM: %w", err)
		}
		dicomtofda.PrintDebug(data, os.Stderr)
	}

	if err := dicomtofda.Convert(inputPath, outputPath, metadataPath); err != nil {
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

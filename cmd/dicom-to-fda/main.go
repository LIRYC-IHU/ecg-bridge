package main

import (
	"fmt"
	"os"

	dicomtofda "converter-fda/dicom-to-fda"

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
	Short: "Convert DICOM ECG files to FDA aECG XML format",
	Long: `dicom-to-fda converts DICOM ECG waveform files (.dcm) into
FDA-compliant annotated ECG XML (aECG) format.

Patient metadata is extracted from the DICOM file by default.
Use --metadata to provide a JSON file that overrides specific fields.

Examples:
  dicom-to-fda --input ecg.dcm --output ecg.xml
  dicom-to-fda --input ecg.dcm --output ecg.xml --metadata patient.json
  dicom-to-fda --input ecg.dcm --debug
  dicom-to-fda --input ecg.dcm | xmllint --format -`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to input DICOM file (.dcm) (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output FDA XML file (default: stdout)")
	rootCmd.Flags().StringVarP(&metadataPath, "metadata", "m", "", "Path to optional patient metadata JSON file")
	rootCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "Print parsed DICOM data to stderr before converting")

	_ = rootCmd.MarkFlagRequired("input")
}

func runConvert(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", inputPath)
	}
	if metadataPath != "" {
		if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
			return fmt.Errorf("metadata file not found: %s", metadataPath)
		}
	}

	dest := outputPath
	if dest == "" {
		dest = "stdout"
	}
	fmt.Fprintf(os.Stderr, "Converting %s → %s\n", inputPath, dest)
	if metadataPath != "" {
		fmt.Fprintf(os.Stderr, "Using metadata from %s\n", metadataPath)
	}

	if debugMode {
		data, err := dicomtofda.ParseDicom(inputPath)
		if err != nil {
			return fmt.Errorf("parsing DICOM: %w", err)
		}
		if metadataPath != "" {
			meta, err := dicomtofda.LoadMetadata(metadataPath)
			if err != nil {
				return fmt.Errorf("reading metadata: %w", err)
			}
			data.MergeMetadata(meta)
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

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"os"

	musetodicom "converter-fda/muse-to-dicom"
	musetofda "converter-fda/muse-to-fda"

	"github.com/spf13/cobra"
)

var (
	inputPath    string
	outputPath   string
	metadataJSON bool
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
	rootCmd.Flags().BoolVar(&metadataJSON, "metadata-json", false, "Output patient metadata as JSON (no waveform)")

	_ = rootCmd.MarkFlagRequired("input")
}

func runConvert(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", inputPath)
	}

	if metadataJSON {
		return runMetadataJSON()
	}

	dest := outputPath
	if dest == "" {
		dest = "stdout"
	}
	fmt.Fprintf(os.Stderr, "Converting %s → %s\n", inputPath, dest)

	if err := musetodicom.Convert(inputPath, outputPath); err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	if outputPath != "" {
		fmt.Fprintf(os.Stderr, "Done. Output written to %s\n", outputPath)
	}
	return nil
}

func runMetadataJSON() error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	d, err := musetofda.ParseMuse(data)
	if err != nil {
		return fmt.Errorf("parsing MUSE XML: %w", err)
	}

	m := map[string]interface{}{
		"patientID":     d.PatientID,
		"patientName":   d.PatientName,
		"gender":        d.PatientSex,
		"age":           d.PatientAge,
		"deviceVersion": d.MuseVersion,
		"sampleRate":    d.SamplingRate,
		"sensitivity":   d.Sensitivity,
		"heartRate":     d.HeartRate,
		"atrialRate":    d.AtrialRate,
		"prInterval":    d.PRInterval,
		"qrsDuration":   d.QRSDuration,
		"qtInterval":    d.QTInterval,
		"qtcInterval":   d.QTcInterval,
		"pAxis":         d.PFrontAxis,
		"qrsAxis":       d.QRSFrontAxis,
		"tAxis":         d.TFrontAxis,
		"leadsCount":    len(d.RhythmLeads),
	}
	if d.StudyDate != "" || d.StudyTime != "" {
		m["datetime"] = d.StudyDate + d.StudyTime
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

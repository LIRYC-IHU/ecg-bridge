package main

import (
	"encoding/json"
	"fmt"
	"os"

	nktodicom "converter-fda/nk-to-dicom"
	nktofda "converter-fda/nk-to-fda"

	"github.com/spf13/cobra"
)

var (
	inputPath    string
	outputPath   string
	debugMode    bool
	metadataJSON bool
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

	fmt.Fprintf(os.Stderr, "Converting %s → %s\n", inputPath, outputPath)

	if err := nktodicom.Convert(inputPath, outputPath); err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Done. DICOM file written to %s\n", outputPath)
	return nil
}

func runMetadataJSON() error {
	dat, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	nd, err := nktofda.ParseFile(dat)
	if err != nil {
		return fmt.Errorf("parsing NK file: %w", err)
	}

	m := map[string]interface{}{
		"patientID":    nd.Patient.PatientID,
		"familyName":   nd.Patient.FamilyName,
		"givenName":    nd.Patient.GivenName,
		"gender":       nd.Patient.Gender,
		"birthDate":    nd.Patient.BirthDate,
		"location":     nd.Patient.Location,
		"deviceModel":  nd.Patient.DeviceModel,
		"heartRate":    nd.Measurement.HeartRate,
		"prInterval":   nd.Measurement.PRInterval,
		"qrsDuration":  nd.Measurement.QRSDuration,
		"qtInterval":   nd.Measurement.QTInterval,
		"qtcInterval":  nd.Measurement.QTcInterval,
		"pAxis":        nd.Measurement.PAxis,
		"qrsAxis":      nd.Measurement.QRSAxis,
		"tAxis":        nd.Measurement.TAxis,
		"v5rAmplitude": nd.Measurement.V5RAmplitude,
		"v1sAmplitude": nd.Measurement.V1SAmplitude,
		"sampleRate":   nd.Record.SampleRate,
		"totalSamples": nd.Record.TotalSamples,
	}
	if !nd.Patient.RecordingAt.IsZero() {
		m["datetime"] = nd.Patient.RecordingAt.Format("20060102150405")
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

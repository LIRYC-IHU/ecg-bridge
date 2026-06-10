package main

import (
	"encoding/json"
	"fmt"
	"os"

	nktofda "converter-fda/nk-to-fda"

	"github.com/spf13/cobra"
)

var (
	inputPath    string
	outputPath   string
	debugMode    bool
	metadataJSON bool
	anonymize    bool
)

var rootCmd = &cobra.Command{
	Use:   "nk-to-fda",
	Short: "Convert Nihon Kohden .DAT files to FDA aECG XML format",
	Long: `nk-to-fda converts Nihon Kohden proprietary ECG files (.DAT / PEC format)
into FDA-compliant HL7 annotated ECG XML (aECG) format.

Examples:
  nk-to-fda --input 00000005.DAT --output ecg.xml
  nk-to-fda --input 00000005.DAT --metadata-json
  nk-to-fda --input 00000005.DAT | xmllint --format -`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to input NK .DAT file (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output FDA XML file (default: stdout)")
	rootCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "Print parsed metadata to stderr")
	rootCmd.Flags().BoolVar(&metadataJSON, "metadata-json", false, "Output patient metadata as JSON (no waveform decoding)")
	rootCmd.Flags().BoolVarP(&anonymize, "anonymize", "a", false, "Strip patient-identifying fields (name, ID, birth date) from the output")

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

	if debugMode {
		printDebug()
	}

	if err := nktofda.Convert(inputPath, outputPath, anonymize); err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	if outputPath != "" {
		fmt.Fprintf(os.Stderr, "Done. Output written to %s\n", outputPath)
	}
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

func printDebug() {
	dat, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "debug: read error: %v\n", err)
		return
	}
	nd, err := nktofda.ParseFile(dat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "debug: parse error: %v\n", err)
		return
	}
	p := nd.Patient
	m := nd.Measurement
	fmt.Fprintf(os.Stderr, "--- NK Metadata ---\n")
	fmt.Fprintf(os.Stderr, "PatientID:    %s\n", p.PatientID)
	fmt.Fprintf(os.Stderr, "Name:         %s %s\n", p.FamilyName, p.GivenName)
	fmt.Fprintf(os.Stderr, "Gender:       %s\n", p.Gender)
	fmt.Fprintf(os.Stderr, "BirthDate:    %s\n", p.BirthDate)
	fmt.Fprintf(os.Stderr, "Location:     %s\n", p.Location)
	fmt.Fprintf(os.Stderr, "DeviceModel:  %s\n", p.DeviceModel)
	if !p.RecordingAt.IsZero() {
		fmt.Fprintf(os.Stderr, "RecordingAt:  %s\n", p.RecordingAt.Format("2006-01-02 15:04:05"))
	}
	fmt.Fprintf(os.Stderr, "SampleRate:   %d Hz\n", nd.Record.SampleRate)
	fmt.Fprintf(os.Stderr, "TotalSamples: %d\n", nd.Record.TotalSamples)
	fmt.Fprintf(os.Stderr, "HeartRate:    %d bpm\n", m.HeartRate)
	fmt.Fprintf(os.Stderr, "PRInterval:   %d ms\n", m.PRInterval)
	fmt.Fprintf(os.Stderr, "QRSDuration:  %d ms\n", m.QRSDuration)
	fmt.Fprintf(os.Stderr, "QTInterval:   %d ms\n", m.QTInterval)
	fmt.Fprintf(os.Stderr, "QTcInterval:  %d ms\n", m.QTcInterval)
	if m.HasPAxis {
		fmt.Fprintf(os.Stderr, "PAxis:        %d°\n", m.PAxis)
	}
	if m.HasQRSAxis {
		fmt.Fprintf(os.Stderr, "QRSAxis:      %d°\n", m.QRSAxis)
	}
	if m.HasTAxis {
		fmt.Fprintf(os.Stderr, "TAxis:        %d°\n", m.TAxis)
	}
	if m.V5RAmplitude > 0 {
		fmt.Fprintf(os.Stderr, "V5R:          %.3f µV\n", m.V5RAmplitude)
	}
	if m.V1SAmplitude > 0 {
		fmt.Fprintf(os.Stderr, "V1S:          %.3f µV\n", m.V1SAmplitude)
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

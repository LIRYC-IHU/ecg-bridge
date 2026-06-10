package main

import (
	"encoding/json"
	"fmt"
	"os"

	mindraytofda "converter-fda/mindray-to-fda"

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
	Use:   "mindray-to-fda",
	Short: "Convert Mindray BeneHeart R12 files to FDA aECG XML format",
	Long: `mindray-to-fda converts Mindray BeneHeart R12 proprietary ECG files
into FDA-compliant HL7 annotated ECG XML (aECG) format.

Examples:
  mindray-to-fda --input 12lead_data_v1 --output ecg.xml
  mindray-to-fda --input 12lead_data_v1 --metadata-json
  mindray-to-fda --input 12lead_data_v1 | xmllint --format -`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to input Mindray file (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output FDA XML file (default: stdout)")
	rootCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "Print parsed metadata to stderr")
	rootCmd.Flags().BoolVar(&metadataJSON, "metadata-json", false, "Output patient metadata as JSON (no waveform)")
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

	if err := mindraytofda.Convert(inputPath, outputPath, anonymize); err != nil {
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
	md, err := mindraytofda.ParseFile(dat)
	if err != nil {
		return fmt.Errorf("parsing Mindray file: %w", err)
	}

	m := map[string]interface{}{
		"patientID":    md.Patient.PatientID,
		"name":         md.Patient.Name,
		"gender":       md.Patient.Gender,
		"paced":        md.Patient.Paced,
		"location":     md.Patient.Location,
		"serialNumber": md.Device.SerialNumber,
		"softwareName": md.Device.SoftwareName,
		"modelName":    md.Device.ModelName,
		"sampleRate":   md.Record.SampleRate,
		"leadsCount":   len(md.Leads),
	}
	if !md.Patient.StartTime.IsZero() {
		m["startTime"] = md.Patient.StartTime.Format("2006-01-02 15:04:05")
	}
	if !md.Patient.EndTime.IsZero() {
		m["endTime"] = md.Patient.EndTime.Format("2006-01-02 15:04:05")
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
	md, err := mindraytofda.ParseFile(dat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "debug: parse error: %v\n", err)
		return
	}
	p := md.Patient
	d := md.Device
	fmt.Fprintf(os.Stderr, "--- Mindray Metadata ---\n")
	fmt.Fprintf(os.Stderr, "PatientID:    %s\n", p.PatientID)
	fmt.Fprintf(os.Stderr, "Name:         %s\n", p.Name)
	fmt.Fprintf(os.Stderr, "Gender:       %s\n", p.Gender)
	fmt.Fprintf(os.Stderr, "Paced:        %v\n", p.Paced)
	fmt.Fprintf(os.Stderr, "Location:     %s\n", p.Location)
	fmt.Fprintf(os.Stderr, "SerialNumber: %s\n", d.SerialNumber)
	fmt.Fprintf(os.Stderr, "Software:     %s\n", d.SoftwareName)
	fmt.Fprintf(os.Stderr, "Model:        %s\n", d.ModelName)
	fmt.Fprintf(os.Stderr, "SampleRate:   %d Hz\n", md.Record.SampleRate)
	fmt.Fprintf(os.Stderr, "Leads:        %d\n", len(md.Leads))
	if !p.StartTime.IsZero() {
		fmt.Fprintf(os.Stderr, "StartTime:    %s\n", p.StartTime.Format("2006-01-02 15:04:05"))
	}
	if !p.EndTime.IsZero() {
		fmt.Fprintf(os.Stderr, "EndTime:      %s\n", p.EndTime.Format("2006-01-02 15:04:05"))
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

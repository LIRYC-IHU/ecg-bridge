package main

import (
	"encoding/json"
	"fmt"
	"os"

	"converter-fda/metaject"
	mindraytodicom "converter-fda/mindray-to-dicom"
	mindraytofda "converter-fda/mindray-to-fda"

	"github.com/spf13/cobra"
)

var (
	inputPath    string
	outputPath   string
	metadataJSON bool
	anonymize    bool
)

var rootCmd = &cobra.Command{
	Use:   "mindray-to-dicom",
	Short: "Convert Mindray BeneHeart R12 files to DICOM ECG format",
	Long: `mindray-to-dicom converts Mindray BeneHeart R12 proprietary ECG files
into DICOM 12-lead ECG Waveform Storage format.

Examples:
  mindray-to-dicom --input 12lead_data_v1 --output ecg.dcm

Inject metadata (JSON on stdin):
  Pipe a JSON object to overwrite patient/acquisition fields before conversion.
  A field present (even "") overwrites; an absent field keeps the file value.
  Keys: patientID, patientName, gender, datetime ("YYYYMMDDHHMMSS")
  echo '{"patientID":"12345","patientName":"DOE^John"}' | mindray-to-dicom -i 12lead_data_v1 -o ecg.dcm`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to input Mindray file (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output DICOM file (optional, defaults to stdout)")
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

	fmt.Fprintf(os.Stderr, "Converting %s → %s\n", inputPath, outputPath)

	meta, err := metaject.FromStdin()
	if err != nil {
		return fmt.Errorf("reading injection metadata from stdin: %w", err)
	}

	if err := mindraytodicom.Convert(inputPath, outputPath, anonymize, meta); err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Done. Output written to %s\n", outputPath)
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

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

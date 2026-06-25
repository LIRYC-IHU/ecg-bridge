package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/LIRYC-IHU/ecg-bridge/metaject"
	nktodicom "github.com/LIRYC-IHU/ecg-bridge/nk-to-dicom"
	nktofda "github.com/LIRYC-IHU/ecg-bridge/nk-to-fda"

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
	Use:   "nk-to-dicom",
	Short: "Convert Nihon Kohden .DAT files to DICOM ECG format",
	Long: `nk-to-dicom converts Nihon Kohden proprietary ECG files (.DAT / PEC format)
into DICOM 12-lead ECG Waveform Storage format (SOP Class 1.2.840.10008.5.1.4.1.1.9.1.1).

Examples:
  nk-to-dicom --input 00000005.DAT --output ecg.dcm
  nk-to-dicom -i patient.DAT -o output.dcm --debug

Inject metadata (JSON on stdin):
  Pipe a JSON object to overwrite patient/recording fields before conversion.
  A field present (even "") overwrites; an absent field keeps the file value.
  Keys: patientID, familyName, givenName (or patientName "LAST^FIRST"), gender,
        birthDate ("YYYYMMDD"), datetime ("YYYYMMDDHHMMSS")
  echo '{"patientID":"12345","familyName":"DOE","givenName":"John"}' | nk-to-dicom -i 00000005.DAT -o ecg.dcm`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to input NK .DAT file (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output DICOM file (required)")
	rootCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "Print debug information to stderr")
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

	if err := nktodicom.Convert(inputPath, outputPath, anonymize, meta); err != nil {
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

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

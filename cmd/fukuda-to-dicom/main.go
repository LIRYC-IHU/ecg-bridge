package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"

	fukudatodicom "github.com/LIRYC-IHU/ecg-bridge/fukuda-to-dicom"
	fukudatofda "github.com/LIRYC-IHU/ecg-bridge/fukuda-to-fda"
	"github.com/LIRYC-IHU/ecg-bridge/metaject"

	"github.com/spf13/cobra"
)

var (
	inputPath    string
	outputPath   string
	metadataJSON bool
	anonymize    bool
)

var rootCmd = &cobra.Command{
	Use:   "fukuda-to-dicom",
	Short: "Convert Fukuda .ECG files to 12-lead ECG DICOM format",
	Long: `fukuda-to-dicom converts Fukuda Denshi proprietary ECG files (.ECG) into
DICOM 12-lead ECG Waveform Storage (1.2.840.10008.5.1.4.1.1.9.1.1).

The waveform is decompressed natively (Huffman bitstream + 2nd-order predictor,
reverse-engineered from paired sample recordings) — no proprietary software or license required.

Examples:
  fukuda-to-dicom --input DATA000.ECG --output ecg.dcm
  echo '{"patientID":"12345","familyName":"DOE"}' | fukuda-to-dicom -i DATA000.ECG -o ecg.dcm`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to input Fukuda .ECG file (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output DICOM file (default: stdout)")
	rootCmd.Flags().BoolVar(&metadataJSON, "metadata-json", false, "Output metadata as JSON (no DICOM output)")
	rootCmd.Flags().BoolVarP(&anonymize, "anonymize", "a", false, "Strip patient-identifying fields from the output")

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

	meta, err := metaject.FromStdin()
	if err != nil {
		return fmt.Errorf("reading injection metadata from stdin: %w", err)
	}

	if err := fukudatodicom.Convert(inputPath, outputPath, anonymize, meta); err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	if outputPath != "" {
		fmt.Fprintf(os.Stderr, "Done. DICOM file written to %s\n", outputPath)
	}
	return nil
}

func runMetadataJSON() error {
	dat, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	fd, err := fukudatofda.ParseFile(dat)
	if err != nil {
		return fmt.Errorf("parsing Fukuda file: %w", err)
	}

	m := map[string]any{
		"patientID":    fd.Patient.PatientID,
		"familyName":   fd.Patient.FamilyName,
		"givenName":    fd.Patient.GivenName,
		"gender":       fd.Patient.Gender,
		"deviceModel":  fd.Patient.DeviceModel,
		"sampleRate":   fd.Record.SampleRate,
		"totalSamples": fd.Record.TotalSamples,
		"numLeads":     fd.Record.NumLeads,
		"scaleUv":      fd.Record.Scale,
		"heartRate":    fd.Measurement.HeartRate,
		"prInterval":   fd.Measurement.PRInterval,
		"qrsDuration":  fd.Measurement.QRSDuration,
		"qtInterval":   fd.Measurement.QTInterval,
		"qtcInterval":  fd.Measurement.QTcInterval,
		"qrsAxis":      fd.Measurement.QRSAxis,
	}
	if !fd.Patient.RecordingAt.IsZero() {
		m["datetime"] = fd.Patient.RecordingAt.Format("20060102150405")
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}

// Version reports the build version.
func Version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	if info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}

func main() {
	rootCmd.Version = Version()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

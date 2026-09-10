package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"

	fukudatofda "github.com/LIRYC-IHU/ecg-bridge/fukuda-to-fda"
	"github.com/LIRYC-IHU/ecg-bridge/metaject"

	"github.com/spf13/cobra"
)

var (
	inputPath    string
	outputPath   string
	debugMode    bool
	metadataJSON bool
	anonymize    bool
	lang         string
)

var rootCmd = &cobra.Command{
	Use:   "fukuda-to-fda",
	Short: "Convert Fukuda .ECG files to FDA aECG XML format",
	Long: `fukuda-to-fda converts Fukuda Denshi proprietary ECG files (.ECG) into
FDA-compliant HL7 annotated ECG XML (aECG) format.

The waveform is decompressed natively (Huffman bitstream + 2nd-order predictor,
reverse-engineered from paired sample recordings) — no proprietary software or license required.

Examples:
  fukuda-to-fda --input DATA000.ECG --output ecg.xml
  fukuda-to-fda --input DATA000.ECG --metadata-json
  fukuda-to-fda --input DATA000.ECG | xmllint --format -

Inject metadata (JSON on stdin):
  echo '{"patientID":"12345","familyName":"DOE","givenName":"John"}' | fukuda-to-fda -i DATA000.ECG -o out.xml`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to input Fukuda .ECG file (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output FDA XML file (default: stdout)")
	rootCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "Print parsed metadata to stderr")
	rootCmd.Flags().BoolVar(&metadataJSON, "metadata-json", false, "Output metadata as JSON (no waveform in output)")
	rootCmd.Flags().BoolVarP(&anonymize, "anonymize", "a", false, "Strip patient-identifying fields from the output")
	rootCmd.Flags().StringVarP(&lang, "lang", "l", "en", "Language for interpretive statement text: en or fr")

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

	meta, err := metaject.FromStdin()
	if err != nil {
		return fmt.Errorf("reading injection metadata from stdin: %w", err)
	}

	if err := fukudatofda.Convert(inputPath, outputPath, anonymize, meta, lang); err != nil {
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
	fd, err := fukudatofda.ParseFile(dat)
	if err != nil {
		return fmt.Errorf("parsing Fukuda file: %w", err)
	}

	m := map[string]interface{}{
		"patientID":    fd.Patient.PatientID,
		"familyName":   fd.Patient.FamilyName,
		"givenName":    fd.Patient.GivenName,
		"gender":       fd.Patient.Gender,
		"birthDate":    fd.Patient.BirthDate,
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

func printDebug() {
	dat, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "debug: read error: %v\n", err)
		return
	}
	fd, err := fukudatofda.ParseFile(dat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "debug: parse error: %v\n", err)
		return
	}
	p := fd.Patient
	fmt.Fprintf(os.Stderr, "--- Fukuda Metadata ---\n")
	fmt.Fprintf(os.Stderr, "Name:         %s %s\n", p.FamilyName, p.GivenName)
	fmt.Fprintf(os.Stderr, "DeviceModel:  %s\n", p.DeviceModel)
	if !p.RecordingAt.IsZero() {
		fmt.Fprintf(os.Stderr, "RecordingAt:  %s\n", p.RecordingAt.Format("2006-01-02 15:04:05"))
	}
	fmt.Fprintf(os.Stderr, "SampleRate:   %d Hz\n", fd.Record.SampleRate)
	fmt.Fprintf(os.Stderr, "TotalSamples: %d\n", fd.Record.TotalSamples)
	fmt.Fprintf(os.Stderr, "NumLeads:     %d\n", fd.Record.NumLeads)
	fmt.Fprintf(os.Stderr, "Scale:        %.2f µV/digit\n", fd.Record.Scale)
	m := fd.Measurement
	fmt.Fprintf(os.Stderr, "HeartRate:    %d bpm\n", m.HeartRate)
	fmt.Fprintf(os.Stderr, "PRInterval:   %d ms\n", m.PRInterval)
	fmt.Fprintf(os.Stderr, "QRSDuration:  %d ms\n", m.QRSDuration)
	fmt.Fprintf(os.Stderr, "QTInterval:   %d ms\n", m.QTInterval)
	fmt.Fprintf(os.Stderr, "QTcInterval:  %d ms\n", m.QTcInterval)
	if m.HasQRSAxis {
		fmt.Fprintf(os.Stderr, "QRSAxis:      %d°\n", m.QRSAxis)
	}
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

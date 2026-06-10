package main

import (
	"encoding/json"
	"fmt"
	"os"

	musetofda "converter-fda/muse-to-fda"

	"github.com/spf13/cobra"
)

var (
	inputPath    string
	outputPath   string
	debugMode    bool
	metadataJSON bool
)

var rootCmd = &cobra.Command{
	Use:   "muse-to-fda",
	Short: "Convert GE MUSE RestingECG XML files to FDA aECG XML format",
	Long: `muse-to-fda converts GE MUSE RestingECG XML files into
FDA-compliant annotated ECG XML (aECG) format.

Examples:
  muse-to-fda --input ecg.xml --output ecg_fda.xml
  muse-to-fda --input ecg.xml --debug
  muse-to-fda --input ecg.xml | xmllint --format -`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to input MUSE RestingECG XML file (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output FDA XML file (default: stdout)")
	rootCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "Print parsed data to stderr before converting")
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

	if debugMode {
		data, err := os.ReadFile(inputPath)
		if err != nil {
			return fmt.Errorf("reading %s: %w", inputPath, err)
		}
		d, err := musetofda.ParseMuse(data)
		if err != nil {
			return fmt.Errorf("parsing MUSE XML: %w", err)
		}
		printDebug(d)
	}

	if err := musetofda.Convert(inputPath, outputPath); err != nil {
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

func printDebug(d *musetofda.MuseData) {
	fmt.Fprintf(os.Stderr, "--- MUSE Data ---\n")
	fmt.Fprintf(os.Stderr, "MuseVersion:  %s\n", d.MuseVersion)
	fmt.Fprintf(os.Stderr, "PatientID:    %s\n", d.PatientID)
	fmt.Fprintf(os.Stderr, "PatientName:  %s\n", d.PatientName)
	fmt.Fprintf(os.Stderr, "PatientSex:   %s\n", d.PatientSex)
	fmt.Fprintf(os.Stderr, "PatientAge:   %s\n", d.PatientAge)
	fmt.Fprintf(os.Stderr, "StudyDate:    %s\n", d.StudyDate)
	fmt.Fprintf(os.Stderr, "StudyTime:    %s\n", d.StudyTime)
	fmt.Fprintf(os.Stderr, "StudyUID:     %s\n", d.StudyUID)
	fmt.Fprintf(os.Stderr, "SamplingRate: %g Hz\n", d.SamplingRate)
	fmt.Fprintf(os.Stderr, "Sensitivity:  %g µV/LSB\n", d.Sensitivity)
	fmt.Fprintf(os.Stderr, "Baseline:     %g\n", d.Baseline)
	fmt.Fprintf(os.Stderr, "FilterHPF:    %g\n", d.FilterHPF)
	fmt.Fprintf(os.Stderr, "FilterLPF:    %g\n", d.FilterLPF)
	fmt.Fprintf(os.Stderr, "NotchFilter:  %g\n", d.NotchFilter)
	fmt.Fprintf(os.Stderr, "HeartRate:    %g bpm\n", d.HeartRate)
	fmt.Fprintf(os.Stderr, "AtrialRate:   %g bpm\n", d.AtrialRate)
	fmt.Fprintf(os.Stderr, "PRInterval:   %g ms\n", d.PRInterval)
	fmt.Fprintf(os.Stderr, "QRSDuration:  %g ms\n", d.QRSDuration)
	fmt.Fprintf(os.Stderr, "QTInterval:   %g ms\n", d.QTInterval)
	fmt.Fprintf(os.Stderr, "QTcInterval:  %g ms\n", d.QTcInterval)
	fmt.Fprintf(os.Stderr, "PFrontAxis:   %g°\n", d.PFrontAxis)
	fmt.Fprintf(os.Stderr, "QRSFrontAxis: %g°\n", d.QRSFrontAxis)
	fmt.Fprintf(os.Stderr, "TFrontAxis:   %g°\n", d.TFrontAxis)
	for i, lead := range d.RhythmLeads {
		fmt.Fprintf(os.Stderr, "Rhythm[%d]: %d samples\n", i, len(lead))
	}
	for i, lead := range d.MedianLeads {
		fmt.Fprintf(os.Stderr, "Median[%d]: %d samples\n", i, len(lead))
	}
	fmt.Fprintf(os.Stderr, "Diagnosis statements: %d\n", len(d.DiagnosisStatements))
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

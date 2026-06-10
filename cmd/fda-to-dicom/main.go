package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	fdatodicom "converter-fda/fda-to-dicom"

	"github.com/spf13/cobra"
)

var (
	inputPath    string
	outputPath   string
	debugMode    bool
	metadataJSON bool
)

var rootCmd = &cobra.Command{
	Use:   "fda-to-dicom",
	Short: "Convert FDA aECG XML files to DICOM ECG format",
	Long: `fda-to-dicom converts FDA HL7 aECG XML files (AnnotatedECG)
into DICOM 12-Lead ECG Waveform Storage (.dcm) format.

Examples:
  fda-to-dicom --input ecg.xml --output ecg.dcm
  fda-to-dicom --input ecg.xml --output ecg.dcm --debug`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to FDA aECG XML file (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output DICOM file (.dcm) (required)")
	rootCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "Print parsed fields to stderr before converting")
	rootCmd.Flags().BoolVar(&metadataJSON, "metadata-json", false, "Output patient metadata as JSON (no waveform)")

	_ = rootCmd.MarkFlagRequired("input")
}

func runConvert(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", inputPath)
	}
	if !strings.HasSuffix(strings.ToLower(inputPath), ".xml") {
		return fmt.Errorf("input must be an FDA aECG XML file (.xml), got: %s", inputPath)
	}

	if metadataJSON {
		return runMetadataJSON()
	}

	if outputPath == "" {
		return fmt.Errorf("output is required (use --output file.dcm)")
	}
	if !strings.HasSuffix(strings.ToLower(outputPath), ".dcm") {
		return fmt.Errorf("output must be a DICOM file (.dcm), got: %s", outputPath)
	}
	if err := fdatodicom.ValidateFDAInput(inputPath); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Converting %s → %s\n", inputPath, outputPath)

	if debugMode {
		data, err := fdatodicom.ParseFDA(inputPath)
		if err != nil {
			return fmt.Errorf("parsing: %w", err)
		}
		printDebug(data)
	}

	if err := fdatodicom.Convert(inputPath, outputPath); err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Done. Output written to %s\n", outputPath)
	return nil
}

func runMetadataJSON() error {
	d, err := fdatodicom.ParseFDA(inputPath)
	if err != nil {
		return fmt.Errorf("parsing FDA XML: %w", err)
	}

	m := map[string]interface{}{
		"patientID":    d.PatientID,
		"patientName":  d.PatientName,
		"gender":       d.PatientSex,
		"birthDate":    d.PatientDOB,
		"age":          d.PatientAge,
		"manufacturer": d.Manufacturer,
		"deviceModel":  d.ModelName,
		"serialNumber": d.SerialNumber,
		"softwareVer":  d.SoftwareVer,
		"location":     d.InstitutionName,
		"sampleRate":   d.SamplingRate,
		"sensitivity":  d.Sensitivity,
		"heartRate":    d.HeartRate,
		"prInterval":   d.PRInterval,
		"qrsDuration":  d.QRSDuration,
		"qtInterval":   d.QTInterval,
		"qtcInterval":  d.QTcInterval,
		"atrialRate":   d.AtrialRate,
		"pAxis":        d.PFrontAxis,
		"qrsAxis":      d.QRSFrontAxis,
		"tAxis":        d.TFrontAxis,
		"leadsCount":   len(d.Leads),
	}
	if d.StudyDate != "" || d.StudyTime != "" {
		m["datetime"] = d.StudyDate + d.StudyTime
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}

func printDebug(d *fdatodicom.FDAData) {
	fmt.Fprintln(os.Stderr, "=== FDA aECG DEBUG ===")
	fmt.Fprintf(os.Stderr, "PatientID:    %s\n", d.PatientID)
	fmt.Fprintf(os.Stderr, "PatientName:  %s\n", d.PatientName)
	fmt.Fprintf(os.Stderr, "PatientSex:   %s\n", d.PatientSex)
	fmt.Fprintf(os.Stderr, "PatientDOB:   %s\n", d.PatientDOB)
	fmt.Fprintf(os.Stderr, "PatientAge:   %s\n", d.PatientAge)
	fmt.Fprintf(os.Stderr, "StudyDate:    %s\n", d.StudyDate)
	fmt.Fprintf(os.Stderr, "StudyTime:    %s\n", d.StudyTime)
	fmt.Fprintf(os.Stderr, "StudyUID:     %s\n", d.StudyUID)
	fmt.Fprintf(os.Stderr, "Institution:  %s\n", d.InstitutionName)
	fmt.Fprintf(os.Stderr, "OperatorID:   %s\n", d.OperatorID)
	fmt.Fprintf(os.Stderr, "Manufacturer: %s\n", d.Manufacturer)
	fmt.Fprintf(os.Stderr, "ModelName:    %s\n", d.ModelName)
	fmt.Fprintf(os.Stderr, "SerialNumber: %s\n", d.SerialNumber)
	fmt.Fprintf(os.Stderr, "SoftwareVer:  %s\n", d.SoftwareVer)
	fmt.Fprintf(os.Stderr, "FilterLPF:    %g Hz\n", d.FilterLPF)
	fmt.Fprintf(os.Stderr, "FilterHPF:    %g Hz\n", d.FilterHPF)
	fmt.Fprintf(os.Stderr, "NotchFilter:  %g Hz\n", d.NotchFilter)
	fmt.Fprintf(os.Stderr, "HeartRate:    %.0f /min\n", d.HeartRate)
	fmt.Fprintf(os.Stderr, "PRInterval:   %.0f ms\n", d.PRInterval)
	fmt.Fprintf(os.Stderr, "QRSDuration:  %.0f ms\n", d.QRSDuration)
	fmt.Fprintf(os.Stderr, "QTInterval:   %.0f ms\n", d.QTInterval)
	fmt.Fprintf(os.Stderr, "QTcInterval:  %.0f ms\n", d.QTcInterval)
	fmt.Fprintf(os.Stderr, "SamplingRate: %g Hz\n", d.SamplingRate)
	fmt.Fprintf(os.Stderr, "Sensitivity:  %g uV/LSB\n", d.Sensitivity)
	fmt.Fprintf(os.Stderr, "Baseline:     %g\n", d.Baseline)
	fmt.Fprintf(os.Stderr, "Leads:        %d", len(d.Leads))
	for _, name := range []string{"I", "II", "III", "aVR", "aVL", "aVF", "V1", "V2", "V3", "V4", "V5", "V6"} {
		if s, ok := d.Leads[name]; ok {
			fmt.Fprintf(os.Stderr, "  %s=%d", name, len(s))
		}
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "RepBeats:     %d leads\n", len(d.RepBeats))
	fmt.Fprintln(os.Stderr, "======================")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

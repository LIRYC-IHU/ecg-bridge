package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/LIRYC-IHU/ecg-bridge/metaject"
	philipstodicom "github.com/LIRYC-IHU/ecg-bridge/philips-to-dicom"

	"github.com/spf13/cobra"
)

var (
	inputPath    string
	outputPath   string
	debugMode    bool
	anonymize    bool
	metadataJSON bool
)

var rootCmd = &cobra.Command{
	Use:   "philips-to-dicom",
	Short: "Convert Philips SierraECG XML files to DICOM ECG format",
	Long: `philips-to-dicom converts Philips SierraECG XML files (v1.03/1.04)
into DICOM 12-Lead ECG Waveform Storage (.dcm) format.

Examples:
  philips-to-dicom --input ecg.xml
  philips-to-dicom --input ecg.xml --output ecg.dcm
  philips-to-dicom --input ecg.xml --output ecg.dcm --debug

Inject metadata (JSON on stdin):
  Pipe a JSON object to overwrite patient/study fields before conversion.
  A field present (even "") overwrites; an absent field keeps the file value.
  Keys: patientID, patientName ("LAST^FIRST"), gender, age, datetime ("YYYYMMDDHHMMSS")
  echo '{"patientID":"12345","patientName":"DOE^John"}' | philips-to-dicom -i ecg.xml -o ecg.dcm`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to Philips SierraECG XML file (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output DICOM file (.dcm) (default: stdout)")
	rootCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "Print parsed fields to stderr before converting")
	rootCmd.Flags().BoolVarP(&anonymize, "anonymize", "a", false, "Strip patient-identifying fields (name, ID, birth date) from the output")
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

	if !strings.HasSuffix(strings.ToLower(inputPath), ".xml") {
		return fmt.Errorf("input must be a Philips SierraECG XML file (.xml), got: %s", inputPath)
	}
	if outputPath != "" && !strings.HasSuffix(strings.ToLower(outputPath), ".dcm") {
		return fmt.Errorf("output must be a DICOM file (.dcm), got: %s", outputPath)
	}

	fmt.Fprintf(os.Stderr, "Converting %s → %s\n", inputPath, outputPath)

	if debugMode {
		data, err := philipstodicom.ParsePhilips(inputPath)
		if err != nil {
			return fmt.Errorf("parsing: %w", err)
		}
		printDebug(data)
	}

	meta, err := metaject.FromStdin()
	if err != nil {
		return fmt.Errorf("reading injection metadata from stdin: %w", err)
	}

	if err := philipstodicom.ConvertWithOptions(inputPath, outputPath, philipstodicom.Options{Anonymize: anonymize, Meta: meta}); err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	if outputPath != "" {
		fmt.Fprintf(os.Stderr, "Done. Output written to %s\n", outputPath)
	}

	return nil
}

func runMetadataJSON() error {
	d, err := philipstodicom.ParsePhilips(inputPath)
	if err != nil {
		return fmt.Errorf("parsing Philips XML: %w", err)
	}

	m := map[string]interface{}{
		"patientID":    d.PatientID,
		"patientName":  d.PatientName,
		"gender":       d.PatientSex,
		"age":          d.PatientAge,
		"manufacturer": d.Manufacturer,
		"deviceModel":  d.ModelName,
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
		"leadsCount":   len(d.RhythmLeads),
	}
	if d.StudyDate != "" || d.StudyTime != "" {
		m["datetime"] = d.StudyDate + d.StudyTime
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}

func printDebug(d *philipstodicom.PhilipsData) {
	fmt.Fprintln(os.Stderr, "=== Philips ECG DEBUG ===")
	fmt.Fprintf(os.Stderr, "PatientID:    %s\n", d.PatientID)
	fmt.Fprintf(os.Stderr, "PatientName:  %s\n", d.PatientName)
	fmt.Fprintf(os.Stderr, "PatientSex:   %s\n", d.PatientSex)
	fmt.Fprintf(os.Stderr, "PatientAge:   %s\n", d.PatientAge)
	fmt.Fprintf(os.Stderr, "StudyDate:    %s\n", d.StudyDate)
	fmt.Fprintf(os.Stderr, "StudyTime:    %s\n", d.StudyTime)
	fmt.Fprintf(os.Stderr, "StudyUID:     %s\n", d.StudyUID)
	fmt.Fprintf(os.Stderr, "Manufacturer: %s\n", d.Manufacturer)
	fmt.Fprintf(os.Stderr, "ModelName:    %s\n", d.ModelName)
	fmt.Fprintf(os.Stderr, "SoftwareVer:  %s\n", d.SoftwareVer)
	fmt.Fprintf(os.Stderr, "Institution:  %s\n", d.InstitutionName)
	fmt.Fprintf(os.Stderr, "SamplingRate: %.0f Hz\n", d.SamplingRate)
	fmt.Fprintf(os.Stderr, "Sensitivity:  %.1f µV/LSB\n", d.Sensitivity)
	fmt.Fprintf(os.Stderr, "FilterHPF:    %g Hz\n", d.FilterHPF)
	fmt.Fprintf(os.Stderr, "FilterLPF:    %g Hz\n", d.FilterLPF)
	fmt.Fprintf(os.Stderr, "NotchFilter:  %g Hz\n", d.NotchFilter)
	fmt.Fprintf(os.Stderr, "RhythmLeads:  12 leads × %d samples\n", len(d.RhythmLeads[0]))
	fmt.Fprintf(os.Stderr, "RepBeats:     %d leads\n", len(d.RepBeats))
	fmt.Fprintf(os.Stderr, "HeartRate:    %.0f /min\n", d.HeartRate)
	fmt.Fprintf(os.Stderr, "PRInterval:   %.0f ms\n", d.PRInterval)
	fmt.Fprintf(os.Stderr, "RRInterval:   %.0f ms\n", d.RRInterval)
	fmt.Fprintf(os.Stderr, "QRSDuration:  %.0f ms\n", d.QRSDuration)
	fmt.Fprintf(os.Stderr, "QTInterval:   %.0f ms\n", d.QTInterval)
	fmt.Fprintf(os.Stderr, "QTcInterval:  %.0f ms\n", d.QTcInterval)
	fmt.Fprintf(os.Stderr, "AtrialRate:   %.0f /min\n", d.AtrialRate)
	fmt.Fprintf(os.Stderr, "PFrontAxis:   %g°\n", d.PFrontAxis)
	fmt.Fprintf(os.Stderr, "QRSFrontAxis: %g°\n", d.QRSFrontAxis)
	fmt.Fprintf(os.Stderr, "TFrontAxis:   %g°\n", d.TFrontAxis)
	fmt.Fprintf(os.Stderr, "STFrontAxis:  %g°\n", d.STFrontAxis)
	fmt.Fprintf(os.Stderr, "QTDispersion: %.0f ms\n", d.QTDispersion)
	fmt.Fprintln(os.Stderr, "=========================")
}

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

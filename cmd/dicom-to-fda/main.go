package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	dicomtofda "github.com/LIRYC-IHU/ecg-bridge/dicom-to-fda"
	"github.com/LIRYC-IHU/ecg-bridge/metaject"

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
	Use:   "dicom-to-fda",
	Short: "Convert DICOM ECG Waveform files to FDA aECG XML format",
	Long: `dicom-to-fda converts DICOM 12-Lead ECG Waveform Storage (.dcm) files
into FDA-compliant annotated ECG XML (aECG) format.

Examples:
  dicom-to-fda --input ecg.dcm --output ecg_fda.xml
  dicom-to-fda --input ecg.dcm --debug
  dicom-to-fda --input ecg.dcm | xmllint --format -

Inject metadata (JSON on stdin):
  Pipe a JSON object to overwrite patient/study fields before conversion.
  A field present (even "") overwrites; an absent field keeps the file value.
  Keys: patientID, patientName ("LAST^FIRST"), gender, age, birthDate ("YYYYMMDD"),
        datetime ("YYYYMMDDHHMMSS")
  echo '{"patientID":"12345","patientName":"DOE^John"}' | dicom-to-fda -i ecg.dcm -o out.xml`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to input DICOM ECG file (.dcm) (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output FDA XML file (default: stdout)")
	rootCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "Print parsed DICOM data to stderr before converting")
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

	if outputPath != "" && !strings.HasSuffix(strings.ToLower(outputPath), ".xml") {
		return fmt.Errorf("output must be an FDA aECG XML file (.xml), got: %s", outputPath)
	}

	dest := outputPath
	if dest == "" {
		dest = "stdout"
	}
	fmt.Fprintf(os.Stderr, "Converting %s → %s\n", inputPath, dest)

	if debugMode {
		data, err := dicomtofda.ParseDicom(inputPath)
		if err != nil {
			return fmt.Errorf("parsing DICOM: %w", err)
		}
		dicomtofda.PrintDebug(data, os.Stderr)
	}

	meta, err := metaject.FromStdin()
	if err != nil {
		return fmt.Errorf("reading injection metadata from stdin: %w", err)
	}

	if err := dicomtofda.Convert(inputPath, outputPath, anonymize, meta); err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	if outputPath != "" {
		fmt.Fprintf(os.Stderr, "Done. Output written to %s\n", outputPath)
	}
	return nil
}

func runMetadataJSON() error {
	d, err := dicomtofda.ParseDicom(inputPath)
	if err != nil {
		return fmt.Errorf("parsing DICOM: %w", err)
	}

	m := map[string]interface{}{
		"patientID":    d.Patient.PatientID,
		"patientName":  d.Patient.PatientName,
		"gender":       d.Patient.PatientSex,
		"birthDate":    d.Patient.PatientBirthDate,
		"age":          d.Patient.PatientAge,
		"manufacturer": d.Patient.Manufacturer,
		"deviceModel":  d.Patient.DeviceModel,
		"serialNumber": d.Patient.DeviceSerial,
		"softwareVer":  d.Patient.SoftwareVersion,
		"location":     d.Patient.InstitutionName,
		"operator":     d.Patient.OperatorsName,
		"studyUID":     d.StudyInstanceUID,
		"waveforms":    len(d.Waveforms),
		"annotations":  len(d.Annotations),
	}
	if d.StudyDate != "" || d.StudyTime != "" {
		m["datetime"] = d.StudyDate + d.StudyTime
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
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

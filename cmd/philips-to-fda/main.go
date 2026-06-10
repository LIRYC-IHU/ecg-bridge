package main

import (
	"fmt"
	"os"

	philipstofda "converter-fda/philips-to-fda"
	philipstodicom "converter-fda/philips-to-dicom"

	"github.com/spf13/cobra"
)

var (
	inputPath  string
	outputPath string
	debugMode  bool
	anonymize  bool
)

var rootCmd = &cobra.Command{
	Use:   "philips-to-fda",
	Short: "Convert Philips SierraECG XML files to FDA aECG XML format",
	Long: `philips-to-fda converts Philips SierraECG XML files (v1.03/1.04)
into FDA-compliant annotated ECG XML (aECG) format.

Examples:
  philips-to-fda --input ecg.xml --output ecg_fda.xml
  philips-to-fda --input ecg.xml --debug
  philips-to-fda --input ecg.xml | xmllint --format -`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to input Philips SierraECG XML file (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output FDA XML file (default: stdout)")
	rootCmd.Flags().BoolVarP(&debugMode, "debug", "d", false, "Print parsed data to stderr before converting")
	rootCmd.Flags().BoolVarP(&anonymize, "anonymize", "a", false, "Strip patient-identifying fields (name, ID, birth date) from the output")

	_ = rootCmd.MarkFlagRequired("input")
}

func runConvert(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", inputPath)
	}

	dest := outputPath
	if dest == "" {
		dest = "stdout"
	}
	fmt.Fprintf(os.Stderr, "Converting %s → %s\n", inputPath, dest)

	if debugMode {
		data, err := philipstodicom.ParsePhilips(inputPath)
		if err != nil {
			return fmt.Errorf("parsing Philips XML: %w", err)
		}
		printDebug(data)
	}

	if err := philipstofda.Convert(inputPath, outputPath, anonymize); err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	if outputPath != "" {
		fmt.Fprintf(os.Stderr, "Done. Output written to %s\n", outputPath)
	}
	return nil
}

func printDebug(d *philipstodicom.PhilipsData) {
	fmt.Fprintf(os.Stderr, "--- Philips Data ---\n")
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
	fmt.Fprintf(os.Stderr, "SamplingRate: %g Hz\n", d.SamplingRate)
	fmt.Fprintf(os.Stderr, "Sensitivity:  %g µV/LSB\n", d.Sensitivity)
	fmt.Fprintf(os.Stderr, "FilterHPF:    %g Hz\n", d.FilterHPF)
	fmt.Fprintf(os.Stderr, "FilterLPF:    %g Hz\n", d.FilterLPF)
	fmt.Fprintf(os.Stderr, "NotchFilter:  %g Hz\n", d.NotchFilter)
	fmt.Fprintf(os.Stderr, "HeartRate:    %g bpm\n", d.HeartRate)
	fmt.Fprintf(os.Stderr, "PRInterval:   %g ms\n", d.PRInterval)
	fmt.Fprintf(os.Stderr, "RRInterval:   %g ms\n", d.RRInterval)
	fmt.Fprintf(os.Stderr, "QRSDuration:  %g ms\n", d.QRSDuration)
	fmt.Fprintf(os.Stderr, "QTInterval:   %g ms\n", d.QTInterval)
	fmt.Fprintf(os.Stderr, "QTcInterval:  %g ms\n", d.QTcInterval)
	fmt.Fprintf(os.Stderr, "AtrialRate:   %g bpm\n", d.AtrialRate)
	fmt.Fprintf(os.Stderr, "PFrontAxis:   %g°\n", d.PFrontAxis)
	fmt.Fprintf(os.Stderr, "QRSFrontAxis: %g°\n", d.QRSFrontAxis)
	fmt.Fprintf(os.Stderr, "TFrontAxis:   %g°\n", d.TFrontAxis)
	fmt.Fprintf(os.Stderr, "STFrontAxis:  %g°\n", d.STFrontAxis)
	fmt.Fprintf(os.Stderr, "QTDispersion: %g ms\n", d.QTDispersion)
	for i, lead := range d.RhythmLeads {
		fmt.Fprintf(os.Stderr, "Lead[%d]: %d samples\n", i, len(lead))
	}
	fmt.Fprintf(os.Stderr, "RepBeats: %d leads\n", len(d.RepBeats))
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

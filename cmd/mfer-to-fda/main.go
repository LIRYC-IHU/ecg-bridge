package main

import (
	"fmt"
	"os"

	"github.com/LIRYC-IHU/ecg-bridge/metaject"
	mfertofda "github.com/LIRYC-IHU/ecg-bridge/mfer-to-fda"

	"github.com/spf13/cobra"
)

var (
	inputPath  string
	outputPath string
	anonymize  bool
)

var rootCmd = &cobra.Command{
	Use:   "mfer-to-fda",
	Short: "Convert MFER (.mwf) files to FDA aECG XML format",
	Long: `mfer-to-fda converts MFER (Medical waveform Format Encoding Rules, .mwf)
ECG files into FDA-compliant HL7 v3 annotated ECG XML (aECG).

Examples:
  mfer-to-fda --input ecg.mwf --output ecg.xml
  mfer-to-fda --input ecg.mwf | xmllint --format -

Inject metadata (JSON on stdin):
  MFER .mwf files often carry no patient identity; pipe a JSON object to fill it.
  Keys: patientID, patientName ("LAST^FIRST"), gender, birthDate ("YYYYMMDD"),
        datetime ("YYYYMMDDHHMMSS")
  echo '{"patientID":"123","patientName":"DOE^John"}' | mfer-to-fda -i ecg.mwf -o out.xml`,
	RunE: runConvert,
}

func init() {
	rootCmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to input MFER .mwf file (required)")
	rootCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output FDA XML file (default: stdout)")
	rootCmd.Flags().BoolVarP(&anonymize, "anonymize", "a", false, "Strip patient-identifying fields from the output")
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

	meta, err := metaject.FromStdin()
	if err != nil {
		return fmt.Errorf("reading injection metadata from stdin: %w", err)
	}

	if err := mfertofda.Convert(inputPath, outputPath, anonymize, meta); err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	if outputPath != "" {
		fmt.Fprintf(os.Stderr, "Done. Output written to %s\n", outputPath)
	}
	return nil
}

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

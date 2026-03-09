package philipstofda_test

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	philipstodicom "converter-fda/philips-to-dicom"
	philipstofda "converter-fda/philips-to-fda"
)

const philipsDataDir = "/Volumes/Signal/ECG/Phillips"

// xmlDir returns the xml_1.03 directory for a patient, or empty if not found.
func xmlDir(patient string) string {
	p := filepath.Join(philipsDataDir, patient, "xml_1.03")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// firstXML returns the first .xml file under a directory.
func firstXML(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".xml") {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// patients returns all patient dirs that have an xml_1.03 directory.
func patients(t *testing.T) []string {
	t.Helper()
	if _, err := os.Stat(philipsDataDir); err != nil {
		t.Skipf("Philips data not available: %v", err)
	}
	entries, err := os.ReadDir(philipsDataDir)
	if err != nil {
		t.Fatalf("reading data dir: %v", err)
	}
	var result []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if xmlDir(e.Name()) != "" {
			result = append(result, e.Name())
		}
	}
	return result
}

// TestConvertAll verifies that every patient converts without error.
func TestConvertAll(t *testing.T) {
	for _, p := range patients(t) {
		xmlFile := firstXML(xmlDir(p))
		if xmlFile == "" {
			continue
		}
		t.Run(p, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), p+".xml")
			if err := philipstofda.Convert(xmlFile, out); err != nil {
				t.Fatalf("Convert: %v", err)
			}
			if _, err := os.Stat(out); err != nil {
				t.Fatalf("output not created: %v", err)
			}
		})
	}
}

// TestConvertOutputIsValidXML checks that the FDA XML output is well-formed.
func TestConvertOutputIsValidXML(t *testing.T) {
	dir := xmlDir("BS1170")
	if dir == "" {
		t.Skip("BS1170 not available")
	}
	xmlFile := firstXML(dir)
	out := filepath.Join(t.TempDir(), "out.xml")
	if err := philipstofda.Convert(xmlFile, out); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	var v interface{}
	if err := xml.Unmarshal(data, &v); err != nil {
		t.Fatalf("output XML is not well-formed: %v", err)
	}
}

// TestAnnotations checks that numeric measurements end up in the FDA XML.
func TestAnnotations(t *testing.T) {
	dir := xmlDir("BS1170")
	if dir == "" {
		t.Skip("BS1170 not available")
	}
	xmlFile := firstXML(dir)
	data, err := philipstodicom.ParsePhilips(xmlFile)
	if err != nil {
		t.Fatalf("ParsePhilips: %v", err)
	}

	out := filepath.Join(t.TempDir(), "out.xml")
	if err := philipstofda.Convert(xmlFile, out); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	content, _ := os.ReadFile(out)
	body := string(content)

	checks := []struct {
		field string
		value float64
	}{
		{"HeartRate", data.HeartRate},
		{"PRInterval", data.PRInterval},
		{"QRSDuration", data.QRSDuration},
		{"QTInterval", data.QTInterval},
		{"QTcInterval", data.QTcInterval},
	}
	for _, c := range checks {
		if c.value == 0 {
			continue
		}
		s := fmt.Sprintf("%.0f", c.value)
		if !strings.Contains(body, s) {
			t.Errorf("%s=%.0f: value %q not found in output", c.field, c.value, s)
		}
	}
}

// TestInterpretationFields checks severity and mdsignatureline appear in the output.
func TestInterpretationFields(t *testing.T) {
	cases := []struct {
		patient     string
		wantSummary string
		wantComment string
	}{
		{"BS1170", "ECG NORMAL", "Unconfirmed Diagnosis"},
		{"BS1016", "ECG PRESQUE NORMAL", "Unconfirmed Diagnosis"},
		{"BS1192", "ECG ANORMAL", "Unconfirmed Diagnosis"},
		{"BS1212", "ECG LIMITE", "Unconfirmed Diagnosis"},
	}

	for _, tc := range cases {
		t.Run(tc.patient, func(t *testing.T) {
			dir := xmlDir(tc.patient)
			if dir == "" {
				t.Skipf("%s not available", tc.patient)
			}
			xmlFile := firstXML(dir)
			out := filepath.Join(t.TempDir(), "out.xml")
			if err := philipstofda.Convert(xmlFile, out); err != nil {
				t.Fatalf("Convert: %v", err)
			}
			content, _ := os.ReadFile(out)
			body := string(content)

			if !strings.Contains(body, "MDC_ECG_INTERPRETATION_SUMMARY") {
				t.Error("missing MDC_ECG_INTERPRETATION_SUMMARY tag")
			}
			if !strings.Contains(body, tc.wantSummary) {
				t.Errorf("summary %q not found in output", tc.wantSummary)
			}
			if !strings.Contains(body, "MDC_ECG_INTERPRETATION_COMMENT") {
				t.Error("missing MDC_ECG_INTERPRETATION_COMMENT tag")
			}
			if !strings.Contains(body, tc.wantComment) {
				t.Errorf("comment %q not found in output", tc.wantComment)
			}
		})
	}
}

// TestParseInterpretationFields checks that ParsePhilips populates interpretation fields.
func TestParseInterpretationFields(t *testing.T) {
	cases := []struct {
		patient     string
		wantSummary string
		wantComment string
	}{
		{"BS1170", "-  ECG NORMAL -", "Unconfirmed Diagnosis"},
		{"BS1016", "- ECG PRESQUE NORMAL  -", "Unconfirmed Diagnosis"},
		{"BS1192", "- ECG ANORMAL  -", "Unconfirmed Diagnosis"},
	}

	for _, tc := range cases {
		t.Run(tc.patient, func(t *testing.T) {
			dir := xmlDir(tc.patient)
			if dir == "" {
				t.Skipf("%s not available", tc.patient)
			}
			xmlFile := firstXML(dir)
			d, err := philipstodicom.ParsePhilips(xmlFile)
			if err != nil {
				t.Fatalf("ParsePhilips: %v", err)
			}
			if d.InterpretationSummary != tc.wantSummary {
				t.Errorf("InterpretationSummary: got %q, want %q", d.InterpretationSummary, tc.wantSummary)
			}
			if d.InterpretationComment != tc.wantComment {
				t.Errorf("InterpretationComment: got %q, want %q", d.InterpretationComment, tc.wantComment)
			}
		})
	}
}

// TestWaveformPresent checks that rhythm waveform data is present in the output.
func TestWaveformPresent(t *testing.T) {
	dir := xmlDir("BS1170")
	if dir == "" {
		t.Skip("BS1170 not available")
	}
	xmlFile := firstXML(dir)
	out := filepath.Join(t.TempDir(), "out.xml")
	if err := philipstofda.Convert(xmlFile, out); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	content, _ := os.ReadFile(out)
	body := string(content)

	// FDA aECG uses digits in waveform data
	if !strings.Contains(body, "<series>") {
		t.Error("expected waveform series in output")
	}
	if !strings.Contains(body, "MDC_ECG_LEAD_I") {
		t.Error("expected lead I in output")
	}
}

// TestFilterValues checks that filter values appear correctly in the output.
func TestFilterValues(t *testing.T) {
	dir := xmlDir("BS1170")
	if dir == "" {
		t.Skip("BS1170 not available")
	}
	xmlFile := firstXML(dir)
	out := filepath.Join(t.TempDir(), "out.xml")
	if err := philipstofda.Convert(xmlFile, out); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	content, _ := os.ReadFile(out)
	body := string(content)

	if !strings.Contains(body, "Low Pass Filter") {
		t.Error("missing Low Pass Filter")
	}
	if !strings.Contains(body, "High Pass Filter") {
		t.Error("missing High Pass Filter")
	}
	if !strings.Contains(body, ">150<") && !strings.Contains(body, `value="150"`) {
		t.Error("LPF value 150 Hz not found")
	}
	if !strings.Contains(body, ">0.05<") && !strings.Contains(body, `value="0.05"`) {
		t.Error("HPF value 0.05 Hz not found")
	}
}

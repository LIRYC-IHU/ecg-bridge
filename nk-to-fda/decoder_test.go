package nktofda

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const (
	testDATFile = "../data_nk/00000005.DAT"
	testFDAFile = "../data_nk/250606392_20250910135412.FDA.xml"
	nSamples    = 5000
)

// parseFDADigits extracts per-lead digit arrays from the FDA XML reference file.
// Returns map[leadName][]int32 for the 8 measured leads.
func parseFDADigits(path string) (map[string][]int32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reCode := regexp.MustCompile(`code="MDC_ECG_LEAD_([^"]+)"`)
	reDigits := regexp.MustCompile(`<digits>(.*?)</digits>`)

	result := make(map[string][]int32)
	var currentLead string

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024) // 4MB line buffer for long digits lines
	for scanner.Scan() {
		line := scanner.Text()
		if m := reCode.FindStringSubmatch(line); m != nil {
			currentLead = m[1]
		}
		if m := reDigits.FindStringSubmatch(line); m != nil && currentLead != "" {
			parts := strings.Fields(m[1])
			samples := make([]int32, 0, len(parts))
			for _, p := range parts {
				v, err := strconv.ParseInt(p, 10, 32)
				if err != nil {
					return nil, fmt.Errorf("parsing digit in lead %s: %w", currentLead, err)
				}
				samples = append(samples, int32(v))
			}
			result[currentLead] = samples
			currentLead = ""
		}
	}
	return result, scanner.Err()
}

func TestDecodeLeads_vs_FDAGroundTruth(t *testing.T) {
	if _, err := os.Stat(testDATFile); os.IsNotExist(err) {
		t.Skipf("test data not found: %s", testDATFile)
	}
	if _, err := os.Stat(testFDAFile); os.IsNotExist(err) {
		t.Skipf("test data not found: %s", testFDAFile)
	}

	dat, err := os.ReadFile(testDATFile)
	if err != nil {
		t.Fatalf("reading .DAT: %v", err)
	}
	secs, err := parseSections(dat)
	if err != nil {
		t.Fatalf("parseSections: %v", err)
	}
	recSec, ok := secs[secRecord]
	if !ok {
		t.Fatal("RECORD section not found")
	}

	// Pass data from section start to EOF so V6 bitstream can read past the nominal boundary
	recData := dat[recSec.offset+14:] // 14 = pecHeaderSize
	decoded, err := DecodeLeads(recData, nSamples)
	if err != nil {
		t.Fatalf("DecodeLeads: %v", err)
	}

	gt, err := parseFDADigits(testFDAFile)
	if err != nil {
		t.Fatalf("parseFDADigits: %v", err)
	}

	measuredLeads := []string{"I", "II", "V1", "V2", "V3", "V4", "V5", "V6"}

	for _, name := range measuredLeads {
		ref, ok := gt[name]
		if !ok {
			t.Errorf("lead %s not found in FDA XML", name)
			continue
		}
		got, ok := decoded[name]
		if !ok {
			t.Errorf("lead %s not decoded", name)
			continue
		}
		if len(ref) != nSamples {
			t.Errorf("lead %s: FDA XML has %d samples, expected %d", name, len(ref), nSamples)
		}
		if len(got) != nSamples {
			t.Errorf("lead %s: decoder produced %d samples, expected %d", name, len(got), nSamples)
		}
		n := min(len(ref), len(got))
		var mismatches int
		for i := 0; i < n; i++ {
			if got[i] != ref[i] {
				if mismatches < 5 {
					t.Errorf("lead %s[%d]: got %d, want %d", name, i, got[i], ref[i])
				}
				mismatches++
			}
		}
		if mismatches == 0 {
			t.Logf("lead %s: %d/%d samples exact match", name, n, nSamples)
		} else {
			t.Errorf("lead %s: %d/%d samples mismatch", name, mismatches, n)
		}
	}
}

func TestParseFile_Metadata(t *testing.T) {
	if _, err := os.Stat(testDATFile); os.IsNotExist(err) {
		t.Skipf("test data not found: %s", testDATFile)
	}

	dat, err := os.ReadFile(testDATFile)
	if err != nil {
		t.Fatalf("reading .DAT: %v", err)
	}
	nd, err := ParseFile(dat)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"PatientID", nd.Patient.PatientID, "250606392"},
		{"FamilyName", nd.Patient.FamilyName, "feugueur"},
		{"Gender", nd.Patient.Gender, "F"},
		{"DeviceModel", nd.Patient.DeviceModel, "2350K"},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}

	if nd.Record.SampleRate != 500 {
		t.Errorf("SampleRate: got %d, want 500", nd.Record.SampleRate)
	}
	if nd.Record.TotalSamples != nSamples {
		t.Errorf("TotalSamples: got %d, want %d", nd.Record.TotalSamples, nSamples)
	}
	if nd.Measurement.HeartRate != 105 {
		t.Errorf("HeartRate: got %d, want 105", nd.Measurement.HeartRate)
	}
	if nd.Measurement.PRInterval != 260 {
		t.Errorf("PRInterval: got %d, want 260", nd.Measurement.PRInterval)
	}
	if nd.Measurement.QRSDuration != 94 {
		t.Errorf("QRSDuration: got %d, want 94", nd.Measurement.QRSDuration)
	}
}

func TestDeriveLeads_vs_FDAGroundTruth(t *testing.T) {
	if _, err := os.Stat(testDATFile); os.IsNotExist(err) {
		t.Skipf("test data not found: %s", testDATFile)
	}
	if _, err := os.Stat(testFDAFile); os.IsNotExist(err) {
		t.Skipf("test data not found: %s", testFDAFile)
	}

	dat, err := os.ReadFile(testDATFile)
	if err != nil {
		t.Fatalf("reading .DAT: %v", err)
	}
	secs, err := parseSections(dat)
	if err != nil {
		t.Fatalf("parseSections: %v", err)
	}
	recSec, ok := secs[secRecord]
	if !ok {
		t.Fatal("RECORD section not found")
	}

	recData := dat[recSec.offset+14:]
	decoded, err := DecodeLeads(recData, nSamples)
	if err != nil {
		t.Fatalf("DecodeLeads: %v", err)
	}

	// Derive 4 augmented leads
	iii, avr, avl, avf := DeriveLeads(decoded["I"], decoded["II"])
	derived := map[string][]int32{
		"III": iii,
		"aVR": avr,
		"aVL": avl,
		"aVF": avf,
	}

	gt, err := parseFDADigits(testFDAFile)
	if err != nil {
		t.Fatalf("parseFDADigits: %v", err)
	}

	for _, name := range []string{"III", "aVR", "aVL", "aVF"} {
		ref, ok := gt[name]
		if !ok {
			t.Errorf("lead %s not found in FDA XML", name)
			continue
		}
		got, ok := derived[name]
		if !ok {
			t.Errorf("lead %s not derived", name)
			continue
		}
		if len(ref) != nSamples {
			t.Errorf("lead %s: FDA XML has %d samples, expected %d", name, len(ref), nSamples)
		}
		if len(got) != nSamples {
			t.Errorf("lead %s: derived %d samples, expected %d", name, len(got), nSamples)
		}
		n := min(len(ref), len(got))
		var mismatches int
		for i := 0; i < n; i++ {
			if got[i] != ref[i] {
				if mismatches < 5 {
					t.Errorf("lead %s[%d]: got %d, want %d", name, i, got[i], ref[i])
				}
				mismatches++
			}
		}
		if mismatches == 0 {
			t.Logf("lead %s: %d/%d samples exact match", name, n, nSamples)
		} else {
			t.Errorf("lead %s: %d/%d samples mismatch", name, mismatches, n)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

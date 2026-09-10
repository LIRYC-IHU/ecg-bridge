package fukudatofda

import (
	"os"
	"testing"
)

// leadIHead is the first 16 samples of lead I decoded from testdata/DATA000.ECG,
// verified byte-exact against the paired reference (MFER) export.
var leadIHead = []int32{-26, -25, -24, -23, -21, -20, -19, -18, -17, -16, -15, -15, -14, -13, -11, -12}

func TestParseDATA000(t *testing.T) {
	dat, err := os.ReadFile("testdata/DATA000.ECG")
	if os.IsNotExist(err) {
		t.Skip("testdata/DATA000.ECG not present (patient recording, not shipped)")
	}
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	fd, err := ParseFile(dat)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if fd.Record.SampleRate != 500 {
		t.Errorf("SampleRate = %d, want 500", fd.Record.SampleRate)
	}
	if fd.Record.TotalSamples != 5000 {
		t.Errorf("TotalSamples = %d, want 5000", fd.Record.TotalSamples)
	}
	if got := fd.Record.Scale; got < 4.87 || got > 4.89 {
		t.Errorf("Scale = %v, want ~4.88", got)
	}
	if fd.Record.NumLeads != 8 {
		t.Errorf("NumLeads = %d, want 8", fd.Record.NumLeads)
	}
	lead := fd.Leads["I"]
	if len(lead) != 5000 {
		t.Fatalf("lead I length = %d, want 5000", len(lead))
	}
	for i, want := range leadIHead {
		if lead[i] != want {
			t.Fatalf("lead I sample %d = %d, want %d", i, lead[i], want)
		}
	}
}

func TestDeriveLeads(t *testing.T) {
	i := []int32{10, 20, 30}
	ii := []int32{40, 60, 80}
	iii, avr, avl, avf := DeriveLeads(i, ii)
	for n := range i {
		if iii[n] != ii[n]-i[n] {
			t.Errorf("III[%d] wrong", n)
		}
		if avr[n] != -(i[n]+ii[n])/2 {
			t.Errorf("aVR[%d] wrong", n)
		}
		if avl[n] != (2*i[n]-ii[n])/2 {
			t.Errorf("aVL[%d] wrong", n)
		}
		if avf[n] != (2*ii[n]-i[n])/2 {
			t.Errorf("aVF[%d] wrong", n)
		}
	}
}

// TestMeasurementsDATA004 checks the analytical measurements against the
// reference values of the paired recording DATA004.ECG.
func TestMeasurementsDATA004(t *testing.T) {
	dat, err := os.ReadFile("testdata/DATA004.ECG")
	if os.IsNotExist(err) {
		t.Skip("testdata/DATA004.ECG not present (patient recording, not shipped)")
	}
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	fd, err := ParseFile(dat)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	m := fd.Measurement
	for _, c := range []struct {
		name       string
		got, want  int
	}{
		{"HeartRate", m.HeartRate, 67},
		{"PRInterval", m.PRInterval, 150},
		{"QRSDuration", m.QRSDuration, 90},
		{"QTInterval", m.QTInterval, 373},
		{"QTcInterval", m.QTcInterval, 394},
		{"QRSAxis", m.QRSAxis, 77},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

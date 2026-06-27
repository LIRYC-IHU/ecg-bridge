package mfertofda

import "testing"

func TestReadLen(t *testing.T) {
	cases := []struct {
		name   string
		buf    []byte
		i      int
		length int
		next   int
		ok     bool
	}{
		{"short form", []byte{0x20}, 0, 0x20, 1, true},
		{"long form 1 byte", []byte{0x81, 0x80}, 0, 0x80, 2, true},
		{"long form 3 bytes (80000)", []byte{0x83, 0x01, 0x38, 0x80}, 0, 80000, 4, true},
		{"truncated long form", []byte{0x82, 0x01}, 0, 0, 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			length, next, ok := readLen(c.buf, c.i)
			if ok != c.ok || (ok && (length != c.length || next != c.next)) {
				t.Fatalf("readLen=%d,%d,%v want %d,%d,%v", length, next, ok, c.length, c.next, c.ok)
			}
		})
	}
}

func TestSamplingRate(t *testing.T) {
	// 01 FD 02 00 → unit=s, exp=-3, mantissa=2 → 2e-3 s → 500 Hz.
	if got := samplingRate([]byte{0x01, 0xFD, 0x02, 0x00}, true); got != 500 {
		t.Fatalf("samplingRate=%v want 500", got)
	}
	if got := samplingRate([]byte{0x01}, true); got != 0 {
		t.Fatalf("samplingRate(short)=%v want 0", got)
	}
}

func TestDecodeWaveformPlanar(t *testing.T) {
	// Two channels, 3 samples each, planar int16 LE: [c0:1,2,3][c1:10,20,30].
	b := []byte{
		1, 0, 2, 0, 3, 0, // channel 0
		10, 0, 20, 0, 30, 0, // channel 1
	}
	leads := decodeWaveform(b, 2, true)
	if got := leads[idxI]; len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("lead I = %v", got)
	}
	if got := leads[idxII]; len(got) != 3 || got[0] != 10 || got[2] != 30 {
		t.Fatalf("lead II = %v", got)
	}
}

func TestDeriveLimbLeads(t *testing.T) {
	var leads [12][]int16
	leads[idxI] = []int16{-104}
	leads[idxII] = []int16{-120}
	deriveLimbLeads(&leads)
	want := map[int]int16{idxIII: -16, idxAVR: 112, idxAVL: -44, idxAVF: -68}
	for idx, w := range want {
		if leads[idx][0] != w {
			t.Errorf("lead idx %d = %d want %d", idx, leads[idx][0], w)
		}
	}
}

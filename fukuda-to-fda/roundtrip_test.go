package fukudatofda

import (
	"errors"
	"testing"
)

// The decode-fidelity tests that compare against real recordings cannot run
// anywhere but a machine holding patient data, so on CI they skip and the suite
// goes green having compared nothing. These tests close that gap: they encode a
// known sample sequence with the format's own Huffman tree and 2nd-order
// predictor, decode it back, and require bit-exact equality. No recording is
// involved, so they run everywhere — and they fail if the tree, the bit reader
// or the predictor is touched.

// leafCodes walks huffTree from the root and returns the bit path to each leaf,
// which is by construction the encoder for that leaf.
func leafCodes(t *testing.T) map[int32][]int {
	t.Helper()
	codes := make(map[int32][]int)

	var walk func(node int32, path []int, depth int)
	walk = func(node int32, path []int, depth int) {
		if depth > 32 {
			t.Fatal("huffTree walk exceeded 32 bits; the tree is not acyclic")
		}
		if node < 0 {
			return // end marker
		}
		if node < huffRoot {
			cp := append([]int(nil), path...)
			codes[node] = cp
			return
		}
		for bit := 0; bit < 2; bit++ {
			walk(huffTree[3*node+int32(bit)], append(path, bit), depth+1)
		}
	}
	walk(huffRoot, nil, 0)

	for leaf := int32(0); leaf <= 12; leaf++ {
		if len(codes[leaf]) == 0 {
			t.Fatalf("leaf %d is unreachable from the root", leaf)
		}
	}
	return codes
}

// bitWriter packs bits MSB-first into big-endian 16-bit words, matching the
// layout bitReader expects.
type bitWriter struct {
	bits []int
}

func (w *bitWriter) write(b ...int) { w.bits = append(w.bits, b...) }

func (w *bitWriter) writeValue(v, n int) {
	for i := n - 1; i >= 0; i-- {
		w.bits = append(w.bits, (v>>uint(i))&1)
	}
}

func (w *bitWriter) bytes() []byte {
	// Pad to a whole 16-bit word; the reader indexes by word.
	for len(w.bits)%16 != 0 {
		w.bits = append(w.bits, 0)
	}
	out := make([]byte, len(w.bits)/8)
	for i, b := range w.bits {
		if b == 1 {
			out[i/8] |= 1 << (7 - uint(i%8))
		}
	}
	return out
}

// encode produces a bitstream that DecodeAll must reconstruct exactly. It is
// the inverse of the decoder: second difference, then the smallest encoding
// that represents it.
func encode(t *testing.T, samples []int32) []byte {
	t.Helper()
	codes := leafCodes(t)
	w := &bitWriter{}
	var prev2, prev1 int32

	for _, x := range samples {
		delta := x - (2*prev1 - prev2)
		switch {
		case delta >= -5 && delta <= 5:
			w.write(codes[delta+5]...)
		case delta >= -128 && delta <= 127:
			w.write(codes[11]...)
			w.writeValue(int(uint8(int8(delta))), 8)
		case delta >= -32768 && delta <= 32767:
			w.write(codes[12]...)
			w.writeValue(int(uint16(int16(delta))), 16)
		default:
			t.Fatalf("sample step %d is not representable in this format", delta)
		}
		prev2, prev1 = prev1, x
	}
	return w.bytes()
}

func TestDecodeAllIsBitExact(t *testing.T) {
	// Values chosen to exercise every branch: flat runs (delta 0), the small
	// -5..+5 deltas, the 8-bit escape and the 16-bit escape, and negatives.
	want := []int32{
		0, 0, 0, 1, 3, 6, 10, 15, 14, 12, 9, 5, 0, -6, -13, -21,
		-20, -18, -15, -11, -6, 0, 200, 500, 900, -400, -1200, 0, 0, 0, 900, 40,
	}
	data := encode(t, want)

	got, err := DecodeAll(data, 0, 1, len(want))
	if err != nil {
		t.Fatalf("DecodeAll: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d leads, want 1", len(got))
	}
	for i := range want {
		if got[0][i] != want[i] {
			t.Fatalf("sample %d = %d, want %d (decode is not bit-exact)", i, got[0][i], want[i])
		}
	}
}

// Leads are stored back to back in one stream and the predictor is NOT reset
// between them. Splitting must therefore not disturb the reconstruction.
func TestDecodeAllSplitsLeadsWithoutResettingThePredictor(t *testing.T) {
	const nLeads, nSamples = 3, 16
	want := make([]int32, 0, nLeads*nSamples)
	v := int32(0)
	for i := 0; i < nLeads*nSamples; i++ {
		v += int32(i%7) - 3
		want = append(want, v)
	}
	data := encode(t, want)

	got, err := DecodeAll(data, 0, nLeads, nSamples)
	if err != nil {
		t.Fatalf("DecodeAll: %v", err)
	}
	for l := 0; l < nLeads; l++ {
		for s := 0; s < nSamples; s++ {
			if got[l][s] != want[l*nSamples+s] {
				t.Fatalf("lead %d sample %d = %d, want %d", l, s, got[l][s], want[l*nSamples+s])
			}
		}
	}
}

// A stream that stops early must be an error. Padding it with zeros produced a
// flat segment that reads, on the trace, exactly like recorded asystole.
func TestDecodeAllRefusesTruncatedStream(t *testing.T) {
	want := []int32{0, 1, 2, 3, 4, 5, 6, 7}
	data := encode(t, want)

	if _, err := DecodeAll(data, 0, 1, len(want)*4); !errors.Is(err, ErrTruncatedStream) {
		t.Fatalf("error = %v, want ErrTruncatedStream", err)
	}
}

func TestDecodeAllHonoursStartBit(t *testing.T) {
	want := []int32{0, 2, 4, 6, 8, 10}
	body := encode(t, want)

	// Prepend one 16-bit word of padding and start the reader past it.
	data := append([]byte{0xAB, 0xCD}, body...)
	got, err := DecodeAll(data, 16, 1, len(want))
	if err != nil {
		t.Fatalf("DecodeAll: %v", err)
	}
	for i := range want {
		if got[0][i] != want[i] {
			t.Fatalf("sample %d = %d, want %d", i, got[0][i], want[i])
		}
	}
}

func TestBitReaderReadsMSBFirstWithinBigEndianWords(t *testing.T) {
	br := &bitReader{data: []byte{0b1011_0010, 0b0100_0001}}
	if got := br.bits(8); got != 0b1011_0010 {
		t.Errorf("first byte = %08b, want 10110010", got)
	}
	if got := br.bits(8); got != 0b0100_0001 {
		t.Errorf("second byte = %08b, want 01000001", got)
	}
}

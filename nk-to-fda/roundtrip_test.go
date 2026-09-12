package nktofda

import (
	"encoding/binary"
	"testing"
)

// The .DAT-versus-reference test in decoder_test.go needs a real recording and
// therefore skips on CI. These tests exercise the same machinery — code table,
// bit reader, Huffman sub-unit, segment reconstruction — on bitstreams built in
// the test, so they run everywhere and fail loudly if the decoder changes.

// --- a minimal, prefix-free code table -------------------------------------
//
// symbol 0  -> "0"     delta  0, no extra bits
// symbol 1  -> "10"    delta -1, no extra bits
// symbol 2  -> "110"   delta +1, no extra bits
// symbol 32 -> "1110"  literal escape, 16 extra bits
type testCode struct {
	symbol    int
	bitLen    int
	firstByte byte
}

var testCodes = []testCode{
	{symbol: 0, bitLen: 1, firstByte: 0b0000_0000},
	{symbol: 1, bitLen: 2, firstByte: 0b1000_0000},
	{symbol: 2, bitLen: 3, firstByte: 0b1100_0000},
	{symbol: 32, bitLen: 4, firstByte: 0b1110_0000},
}

// buildCodeTable serialises testCodes in the on-disk layout parseCodeTable
// expects: a u16 size, then per symbol a length byte followed by ceil(len/8)
// codeword bytes.
func buildCodeTable() []byte {
	lengths := map[int]testCode{}
	for _, c := range testCodes {
		lengths[c.symbol] = c
	}
	body := []byte{}
	for i := 0; i < 33; i++ {
		c, ok := lengths[i]
		if !ok {
			body = append(body, 0)
			continue
		}
		body = append(body, byte(c.bitLen))
		body = append(body, c.firstByte) // every test code fits in one byte
	}
	out := make([]byte, 2, 2+len(body))
	binary.BigEndian.PutUint16(out, uint16(len(body)+2))
	return append(out, body...)
}

type nkBitWriter struct{ bits []int }

func (w *nkBitWriter) writeValue(v, n int) {
	for i := n - 1; i >= 0; i-- {
		w.bits = append(w.bits, (v>>uint(i))&1)
	}
}

// symbol writes one Huffman codeword, plus its literal payload for symbol 32.
func (w *nkBitWriter) symbol(sym int, literal int) {
	for _, c := range testCodes {
		if c.symbol != sym {
			continue
		}
		w.writeValue(int(c.firstByte>>(8-uint(c.bitLen))), c.bitLen)
		if extraBitsCount[sym] > 0 {
			w.writeValue(literal&0xFFFF, extraBitsCount[sym])
		}
		return
	}
	panic("unknown test symbol")
}

// bytes returns the sub-unit: a u32 bit count (counted from the start of the
// sub-unit, header included) followed by the packed bitstream.
func (w *nkBitWriter) bytes() []byte {
	total := uint32(32 + len(w.bits))
	payload := make([]byte, (len(w.bits)+7)/8+4)
	binary.BigEndian.PutUint32(payload, total)
	for i, b := range w.bits {
		if b == 1 {
			payload[4+i/8] |= 1 << (7 - uint(i%8))
		}
	}
	return payload
}

func TestParseCodeTableRoundTrip(t *testing.T) {
	syms, err := parseCodeTable(buildCodeTable(), 0)
	if err != nil {
		t.Fatalf("parseCodeTable: %v", err)
	}
	for _, c := range testCodes {
		got := syms[c.symbol]
		if got.bitLen != c.bitLen {
			t.Errorf("symbol %d bitLen = %d, want %d", c.symbol, got.bitLen, c.bitLen)
		}
		want := uint32(c.firstByte) << 24
		if got.codeword != want {
			t.Errorf("symbol %d codeword = %#08x, want %#08x", c.symbol, got.codeword, want)
		}
	}
	// Symbols with no code must stay zero-length, or the decoder would match
	// them against an all-zero mask and consume the stream at random.
	for i := 3; i < 32; i++ {
		if syms[i].bitLen != 0 {
			t.Errorf("symbol %d has bitLen %d, want 0", i, syms[i].bitLen)
		}
	}
}

func TestDecodeSubunitRecoversSymbolsAndLiterals(t *testing.T) {
	syms, err := parseCodeTable(buildCodeTable(), 0)
	if err != nil {
		t.Fatalf("parseCodeTable: %v", err)
	}

	w := &nkBitWriter{}
	w.symbol(0, 0)      // delta 0
	w.symbol(2, 0)      // delta +1
	w.symbol(1, 0)      // delta -1
	w.symbol(32, 1234)  // literal 1234
	w.symbol(32, -1234) // literal -1234, sign-extended from 16 bits
	w.symbol(0, 0)

	want := []int{0, 1, -1, 1234, -1234, 0}
	got, bits := decodeSubunit(w.bytes(), 0, syms, len(want)+4)
	if bits == 0 {
		t.Fatal("decodeSubunit reported a zero bit count")
	}
	if len(got) != len(want) {
		t.Fatalf("decoded %d samples (%v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sample %d = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestReadBitsIsMSBFirst(t *testing.T) {
	data := []byte{0b1011_0010, 0b0100_0001}
	if got := readBits(data, 0, 8); got != 0b1011_0010 {
		t.Errorf("readBits(0,8) = %08b, want 10110010", got)
	}
	if got := readBits(data, 4, 8); got != 0b0010_0100 {
		t.Errorf("readBits(4,8) = %08b, want 00100100", got)
	}
	// Past the end returns what it could read rather than reading out of bounds.
	if got := readBits(data, 8, 16); got != 0b0100_0001 {
		t.Errorf("readBits(8,16) = %b, want 01000001", got)
	}
}

func TestS16TruncWrapsAtSixteenBits(t *testing.T) {
	for _, tc := range []struct {
		in   int
		want int32
	}{
		{0, 0},
		{32767, 32767},
		{-32768, -32768},
		{32768, -32768}, // wraps, as the format's arithmetic does
		{65535, -1},
		{65536, 0},
	} {
		if got := s16trunc(tc.in); got != tc.want {
			t.Errorf("s16trunc(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// --- segment reconstruction ------------------------------------------------

// Mode 0 writes the decoded samples through unchanged. This is the path that
// makes "the samples are those of the source file" true.
func TestMode0WriteIsVerbatim(t *testing.T) {
	const lead, nCh = 0, 8
	out := make([]int32, nCh*16)
	seg := []int{10, -20, 30, -40, 50}

	end := mode0Write(out, 16, seg, len(seg), lead, 0)
	if end != len(seg) {
		t.Fatalf("mode0Write returned %d, want %d", end, len(seg))
	}
	for i, want := range seg {
		if got := out[i*nCh+lead]; got != int32(want) {
			t.Errorf("sample %d = %d, want %d", i, got, want)
		}
	}
}

// Mode 1 does NOT write samples through: the format stores those segments at
// half rate and the decoder reconstructs the intermediate samples by linear
// interpolation. Those values are computed here, not read from the file.
//
// This test exists to keep that fact explicit and stable. It is the one place
// in the decode path where a sample on the output did not come from the input,
// and any statement about decode fidelity has to account for it.
func TestMode1UpsampleSynthesisesMidpoints(t *testing.T) {
	const lead, nCh = 0, 8
	out := make([]int32, nCh*32)
	seg := []int{10, 20, 30, 40, 50, 60}

	end := mode1Upsample(out, 32, seg, len(seg), lead, 0, 0)

	// Stored samples land on even positions; position 1 is interpolated.
	want := []int32{10, 15, 20, 30, 40, 50, 60}
	for i, w := range want {
		if got := out[i*nCh+lead]; got != w {
			t.Errorf("position %d = %d, want %d (full output %v)", i, got, w, out[:len(want)*nCh])
		}
	}
	if end != len(want) {
		t.Errorf("mode1Upsample returned %d, want %d", end, len(want))
	}

	// The interpolated sample is the midpoint of its neighbours, not a repeat.
	if out[1*nCh+lead] != (out[0*nCh+lead]+out[2*nCh+lead])/2 {
		t.Error("interpolated sample is not the midpoint of its neighbours")
	}
}

func TestDeriveLeadsFollowsEinthovenAndGoldberger(t *testing.T) {
	i := []int32{100, -40, 0}
	ii := []int32{60, 20, -30}

	iii, avr, avl, avf := DeriveLeads(i, ii)
	for n := range i {
		if want := ii[n] - i[n]; iii[n] != want {
			t.Errorf("III[%d] = %d, want %d", n, iii[n], want)
		}
		if want := -(i[n] + ii[n]) / 2; avr[n] != want {
			t.Errorf("aVR[%d] = %d, want %d", n, avr[n], want)
		}
		if want := (2*i[n] - ii[n]) / 2; avl[n] != want {
			t.Errorf("aVL[%d] = %d, want %d", n, avl[n], want)
		}
		if want := (2*ii[n] - i[n]) / 2; avf[n] != want {
			t.Errorf("aVF[%d] = %d, want %d", n, avf[n], want)
		}
	}
}

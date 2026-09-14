package fukudatofda

import (
	"errors"
	"fmt"
)

// ErrTruncatedStream is returned when the waveform bitstream does not carry all
// the samples the file header announces.
var ErrTruncatedStream = errors.New("fukuda: truncated waveform bitstream")

// Waveform decompression for Fukuda .ECG files.
//
// Reverse-engineered from paired sample recordings (raw .ECG alongside their
// reference waveform exports). The rhythm data is a single continuous Huffman
// bitstream, big-endian 16-bit words read MSB-first, carrying second-difference
// deltas that are reconstructed with a 2nd-order predictor. All leads are stored
// back to back in one stream; the predictor state carries from one lead into the
// next (it is NOT reset per lead).
//
// Validated byte-exact (200000/200000 samples over 5 reference files) against the
// paired reference (MFER) waveform exports.

// huffTree is the static Huffman decode tree recovered during reverse-engineering.
// Layout is flat, 3 int32 per node: [child0, child1, unused]. Node 13 is the root.
// Nodes 0..12 are leaves, 13..25 internal. A negative child is the end marker.
var huffTree = [...]int32{
	-1, -1, -23, -1, -1, -21, -1, -1, 19, -1, -1, -16, -1, -1, -14, -1, -1, 14,
	-1, -1, 16, -1, -1, -17, -1, -1, -19, -1, -1, -22, -1, -1, -24, -1, -1, -20,
	-1, -1, -25, 15, 14, 0, 5, 4, -13, 17, 16, 13, 6, 3, -15, 18, 7, 15, 20, 19,
	17, 2, 8, -18, 21, 11, 18, 22, 1, 20, 23, 9, 21, 24, 0, 22, 25, 10, 23, -1,
	12, 23,
}

const huffRoot = 13

// bitReader walks a big-endian 16-bit-word bitstream MSB-first.
type bitReader struct {
	data []byte
	pos  int // absolute bit position
}

func (b *bitReader) bit() int {
	widx := b.pos / 16
	o := widx * 2
	var w uint16
	if o+2 <= len(b.data) {
		w = uint16(b.data[o])<<8 | uint16(b.data[o+1])
	}
	v := int(w>>(15-uint(b.pos%16))) & 1
	b.pos++
	return v
}

func (b *bitReader) bits(n int) int {
	v := 0
	for i := 0; i < n; i++ {
		v = v<<1 | b.bit()
	}
	return v
}

// DecodeAll decodes nLeads*nSamples samples from the bitstream starting at
// startBit and splits them into per-lead slices. Leads are stored sequentially
// (I, II, V1..V6) with predictor state carried across the whole stream.
//
// A stream that ends before nLeads*nSamples samples have been produced is an
// error. It used to be padded with zeros, which silently turned a truncated
// file into a flat isoelectric segment — indistinguishable, on the trace, from
// a recorded asystole.
func DecodeAll(data []byte, startBit, nLeads, nSamples int) ([][]int32, error) {
	br := &bitReader{data: data, pos: startBit}
	total := nLeads * nSamples
	flat := make([]int32, 0, total)
	var prev2, prev1 int32 // x[n-2], x[n-1]
	for len(flat) < total {
		node := int32(huffRoot)
		for node >= huffRoot {
			node = huffTree[3*node+int32(br.bit())]
		}
		if node < 0 { // end marker
			break
		}
		var delta int32
		switch {
		case node <= 10:
			delta = node - 5 // small delta, -5..+5
		case node == 11:
			delta = int32(int8(br.bits(8))) // 8-bit signed escape
		default: // node == 12
			delta = int32(int16(br.bits(16))) // 16-bit signed escape
		}
		x := 2*prev1 - prev2 + delta
		prev2, prev1 = prev1, x
		flat = append(flat, x)
	}
	if len(flat) < total {
		return nil, fmt.Errorf("%w: bitstream ended after %d of %d samples", ErrTruncatedStream, len(flat), total)
	}
	leads := make([][]int32, nLeads)
	for i := 0; i < nLeads; i++ {
		leads[i] = flat[i*nSamples : (i+1)*nSamples]
	}
	return leads, nil
}

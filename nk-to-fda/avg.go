package nktofda

import "encoding/binary"

// avgTemplates holds the per-lead median-beat templates and the subtraction-zone
// parameters needed to reconstruct the QRS regions of the rhythm signal.
//
// In NK rhythm encoding, the QRS complexes are removed before compression (only
// the inter-beat residual is stored) and must be re-added at decode time using
// the averaged median beat. Segments flagged as mode 2 or 3 carry such zones.
type avgTemplates struct {
	tpl       [8][]int32 // per-lead median beat, 600 samples, scaled and aligned for indexing
	subOffset int        // template index where pre-QRS reconstruction starts (mode 2)
	zoneMid   int        // QRS-zone midpoint (also the mode-3 start)
	zoneEnd   int        // QRS-zone end
}

// avgU16 / avgU32 read big-endian integers with bounds checking.
func avgU16(d []byte, off int) int {
	if off < 0 || off+2 > len(d) {
		return 0
	}
	return int(binary.BigEndian.Uint16(d[off : off+2]))
}

func avgU32(d []byte, off int) int {
	if off < 0 || off+4 > len(d) {
		return 0
	}
	return int(binary.BigEndian.Uint32(d[off : off+4]))
}

// findAvgBase locates the median-beat sub-header by its stable field signature:
// number of leads (8) at +6, sample rate (500 Hz) at +8, and the second-difference
// encoding marker (2) at +10. Returns -1 if not present.
func findAvgBase(dat []byte) int {
	for sb := 0; sb < len(dat)-200; sb++ {
		if avgU16(dat, sb+6) == 8 && avgU16(dat, sb+8) == 500 && avgU16(dat, sb+10) == 2 {
			return sb
		}
	}
	return -1
}

// buildAvgTemplates decodes the per-lead median beat and the subtraction-zone
// parameters. Returns nil when the median sub-header is absent or malformed
// (recordings without averaged beats use only segment modes 0/1 and need none).
//
// Header layout (relative to the sub-header base, big-endian):
//
//	+16  valid-region start of the 600-sample median buffer (constant 48)
//	+22  number of averaged beats (sizes the Huffman-table offset)
//	+28  samples before the QRS peak in the template
//	+32  QRS-zone midpoint
//	+34  QRS-zone end
//
// Each lead's median is a Huffman-compressed second-difference stream; double
// integration yields the waveform, placed in the buffer at [start+1 ..].
func buildAvgTemplates(dat []byte) *avgTemplates {
	sb := findAvgBase(dat)
	if sb < 0 {
		return nil
	}
	avgStart := avgU16(dat, sb+16)
	nBeats := avgU16(dat, sb+22)
	preQRS := avgU16(dat, sb+28)
	subMid := avgU16(dat, sb+32)
	subEnd := avgU16(dat, sb+34)

	// The Huffman table and the lead blocks sit after a beat table whose length
	// depends on the beat count.
	huffOff := 48 + 4*nBeats
	tableSize := avgU16(dat, sb+huffOff)
	if tableSize == 0 {
		return nil
	}
	syms, err := parseCodeTable(dat, sb+huffOff)
	if err != nil {
		return nil
	}
	blockDataStart := sb + huffOff + tableSize + 2

	at := &avgTemplates{subOffset: preQRS, zoneMid: subMid, zoneEnd: subEnd}
	pos := 0
	for li := 0; li < 8; li++ {
		dec, _ := decodeSubunit(dat, blockDataStart+pos, syms, 600)

		// Double-integrate the second-difference stream into the 600-sample buffer.
		buf := make([]int32, 600)
		c1, c2 := 0, 0
		off := avgStart + 1
		for k := 0; k < len(dec); k++ {
			c1 += dec[k]
			c2 += c1
			if idx := off + k; idx >= 0 && idx < 600 {
				buf[idx] = int32(2 * c2)
			}
		}

		// The QRS reconstruction indexes the template one sample ahead and applies
		// a ×2 amplitude scale; bake both in so the handler can index directly.
		tpl := make([]int32, 600)
		for i := 0; i < 599; i++ {
			tpl[i] = 2 * buf[i+1]
		}
		at.tpl[li] = tpl

		// Advance to the next lead block: its bit length rounds up to whole words.
		hb := avgU32(dat, blockDataStart+pos)
		blockBytes := ((hb + 15) / 16) * 2
		if blockBytes <= 0 {
			break
		}
		pos += blockBytes
	}
	return at
}

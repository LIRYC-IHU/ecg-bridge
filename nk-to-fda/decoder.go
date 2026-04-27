package nktofda

import (
	"encoding/binary"
	"fmt"
)

var deltaTable = [33]int{
	0, -1, 1, -2, 2, -3, 3, -4, 4, -5, 5, -6, 6, -7, 7, -8, 8,
	-9, 9, -10, 10, -11, 11, -12, 12, -13, 13, -29, 29, -45, 45, -301, 0,
}

var extraBitsCount = [33]int{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4, 4, 4, 4, 8, 8, 16,
}

// huffSym holds a Huffman code table entry.
type huffSym struct {
	bitLen  int
	codeword uint32 // MSB-aligned on 32 bits
}

// DecodeLeads decodes all 8 measured leads from the RECORD section data.
// rec = RECORD section data (starts at section_offset + 14 in the file).
// nSamples = total samples per lead (e.g. 5000).
func DecodeLeads(rec []byte, nSamples int) (map[string][]int32, error) {
	if len(rec) < 4 {
		return nil, fmt.Errorf("RECORD data too short")
	}
	compType := int(binary.BigEndian.Uint16(rec[4:6]))
	if compType != 16 {
		return nil, fmt.Errorf("unsupported compression type %d (only type 16 supported)", compType)
	}

	// Extract mode/param flags from Gap frame header @ rec[0x0AA2]
	nSegs, modes, params, err := extractModesParams(rec, 0x0AA2)
	if err != nil {
		return nil, fmt.Errorf("extracting mode flags: %w", err)
	}

	// Frame definitions: (leadName, frame_start_in_rec, lead_index)
	// Gap frame: frame_start = 0x0AB4 (ct_off = 0x0AB6)
	// F1-F7: use frame_offsets helper
	type frameSpec struct {
		name       string
		frameStart int // used for F1-F7 to compute ct_off
		ctOff      int // explicit ct_off for Gap
		leadIdx    int
	}

	// Gap frame uses special ct_off (0x0AB6) since first 20 bytes are the flags header.
	gapCtOff := 0x0AB6
	gapCtSize := int(binary.BigEndian.Uint16(rec[gapCtOff : gapCtOff+2]))
	gapDataOff := gapCtOff + 2 + gapCtSize

	frames := []frameSpec{
		{name: "I", ctOff: gapCtOff, leadIdx: 0},
	}
	for _, f := range []struct {
		name       string
		frameStart int
		leadIdx    int
	}{
		{"II", 0x1A66, 1},
		{"V1", 0x29E4, 2},
		{"V2", 0x359A, 3},
		{"V3", 0x417A, 4},
		{"V4", 0x4D5A, 5},
		{"V5", 0x594A, 6},
		{"V6", 0x6560, 7},
	} {
		ctOff := f.frameStart + 2
		frames = append(frames, frameSpec{
			name:       f.name,
			frameStart: f.frameStart,
			ctOff:      ctOff,
			leadIdx:    f.leadIdx,
		})
	}
	_ = gapDataOff // used inline below

	result := make(map[string][]int32, 8)
	for _, fr := range frames {
		var dataOff int
		if fr.name == "I" {
			dataOff = gapDataOff
		} else {
			ctSize := int(binary.BigEndian.Uint16(rec[fr.ctOff : fr.ctOff+2]))
			dataOff = fr.ctOff + 2 + ctSize
		}
		samples, err := decodeLeadFrame(rec, fr.ctOff, dataOff, nSegs, modes, params, fr.leadIdx, nSamples)
		if err != nil {
			return nil, fmt.Errorf("decoding lead %s: %w", fr.name, err)
		}
		result[fr.name] = samples
	}
	return result, nil
}

// extractModesParams extracts n_segments, modes[], params[] from the Gap frame header.
func extractModesParams(rec []byte, sectionStart int) (int, []int, []int, error) {
	if sectionStart+18 > len(rec) {
		return 0, nil, nil, fmt.Errorf("Gap frame header out of bounds")
	}
	bitCount := int(binary.BigEndian.Uint16(rec[sectionStart+2 : sectionStart+4]))
	nSegments := bitCount >> 2

	au898 := [13]int{0, 0}
	for i := 2; i <= 8; i++ {
		off := sectionStart + i*2
		if off+2 <= len(rec) {
			au898[i] = int(binary.BigEndian.Uint16(rec[off : off+2]))
		}
	}

	modes := make([]int, nSegments)
	params := make([]int, nSegments)
	for i := 0; i < nSegments; i++ {
		uVar8 := i * 4
		wordIdx := (uVar8 >> 4) + 2
		if wordIdx >= len(au898) {
			wordIdx = len(au898) - 1
		}
		uVar14 := au898[wordIdx]
		cVar3 := (-(uVar8 & 0xf)) & 0xff
		shiftMode := (cVar3 + 14) & 0x1f
		shiftParam := (cVar3 + 12) & 0x1f
		modes[i] = (uVar14 >> uint(shiftMode)) & 3
		params[i] = (uVar14 >> uint(shiftParam)) & 3
	}
	return nSegments, modes, params, nil
}

// parseCodeTable parses a per-frame Huffman code table.
// Returns symbols[33] and advances past the table.
func parseCodeTable(rec []byte, offset int) ([33]huffSym, error) {
	var syms [33]huffSym
	if offset+2 > len(rec) {
		return syms, fmt.Errorf("CT offset out of bounds")
	}
	pos := offset + 2 // skip total_size u16
	for i := 0; i < 33; i++ {
		if pos >= len(rec) {
			break
		}
		bl := int(rec[pos])
		pos++
		if bl == 0 {
			syms[i] = huffSym{}
			continue
		}
		cb := (bl + 7) / 8
		if pos+cb > len(rec) {
			return syms, fmt.Errorf("CT codeword out of bounds at symbol %d", i)
		}
		var code uint32
		for b := 0; b < cb; b++ {
			code = (code << 8) | uint32(rec[pos+b])
		}
		code <<= uint(32 - 8*cb)
		syms[i] = huffSym{bitLen: bl, codeword: code}
		pos += cb
	}
	return syms, nil
}

// readWindow32 reads a 32-bit window starting at bit_pos (MSB-first).
func readWindow32(rec []byte, bitPos int) uint32 {
	byteIdx := bitPos / 8
	bitOff := bitPos % 8
	var val uint64
	for b := 0; b < 5; b++ {
		idx := byteIdx + b
		var byt byte
		if idx < len(rec) {
			byt = rec[idx]
		}
		val = (val << 8) | uint64(byt)
	}
	return uint32((val >> uint(8-bitOff)) & 0xFFFFFFFF)
}

// readBits reads count bits at bit_pos (MSB-first). Returns 0 for bits past end of slice.
func readBits(rec []byte, bitPos, count int) int {
	result := 0
	for i := 0; i < count; i++ {
		bp := bitPos + i
		byteIdx := bp / 8
		if byteIdx >= len(rec) {
			break
		}
		bit := int(rec[byteIdx]>>(7-uint(bp%8))) & 1
		result = (result << 1) | bit
	}
	return result
}

// decodeSubunit decodes one Huffman-compressed sub-unit.
// Returns (samples, total_bits_including_u32_header).
func decodeSubunit(rec []byte, byteOffset int, syms [33]huffSym, maxSamp int) ([]int, uint32) {
	if byteOffset+4 > len(rec) {
		return nil, 0
	}
	totalBits := binary.BigEndian.Uint32(rec[byteOffset : byteOffset+4])
	if totalBits == 0 || totalBits > 200000 {
		return nil, totalBits
	}
	bitPos := byteOffset*8 + 32
	maxBit := byteOffset*8 + int(totalBits)
	samples := make([]int, 0, maxSamp)

	for s := 0; s < maxSamp; s++ {
		if bitPos >= maxBit {
			break
		}
		window := readWindow32(rec, bitPos)
		found := false
		for i := 0; i < 33; i++ {
			bl := syms[i].bitLen
			if bl == 0 {
				continue
			}
			mask := (uint32(0xFFFFFFFF) << uint(32-bl)) & 0xFFFFFFFF
			if (window & mask) == syms[i].codeword {
				bitPos += bl
				eb := extraBitsCount[i]
				var sample int
				if eb > 0 {
					extra := readBits(rec, bitPos, eb)
					bitPos += eb
					raw := (extra + deltaTable[i]) & 0xFFFF
					if raw >= 0x8000 {
						sample = raw - 0x10000
					} else {
						sample = raw
					}
				} else {
					sample = deltaTable[i]
				}
				samples = append(samples, sample)
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	return samples, totalBits
}

// decodeLeadFrame decodes one lead from its frame.
func decodeLeadFrame(rec []byte, ctOff, dataStart, nSegments int, modes, params []int, leadIdx, nSamples int) ([]int32, error) {
	syms, err := parseCodeTable(rec, ctOff)
	if err != nil {
		return nil, err
	}

	// 1. Huffman decode all sub-units
	leadOff := dataStart
	var allRaw []int
	chunkCounts := make([]int, 0, nSegments)

	for si := 0; si < nSegments; si++ {
		if leadOff >= len(rec) {
			break
		}
		samps, tbits := decodeSubunit(rec, leadOff, syms, 3000)
		if samps == nil || len(samps) == 0 {
			break
		}
		chunkCounts = append(chunkCounts, len(samps))
		allRaw = append(allRaw, samps...)
		bytesUsed := (int(tbits) + 7) / 8
		nextOff := leadOff + bytesUsed
		if nextOff%2 != 0 {
			nextOff++
		}
		leadOff = nextOff
	}

	// 2. Global cumsum
	buf := make([]int, len(allRaw))
	copy(buf, allRaw)
	for i := 1; i < len(buf); i++ {
		buf[i] += buf[i-1]
	}

	// 3. Per-segment <<3 + mode dispatch
	output := make([]int32, nSamples*8)
	outPos := 0
	bufOffset := 0
	for segIdx := 0; segIdx < len(chunkCounts); segIdx++ {
		if segIdx >= nSegments {
			break
		}
		mode := modes[segIdx]
		param := params[segIdx]
		nChunk := chunkCounts[segIdx]
		segVals := make([]int, nChunk)
		for i := 0; i < nChunk; i++ {
			segVals[i] = buf[bufOffset+i] << 3
		}
		bufOffset += nChunk

		var newPos int
		switch mode {
		case 0:
			newPos = mode0Write(output, nSamples, segVals, nChunk, leadIdx, outPos)
		case 1:
			newPos = mode1Upsample(output, nSamples, segVals, nChunk, leadIdx, outPos, param)
		default:
			newPos = mode0Write(output, nSamples, segVals, nChunk, leadIdx, outPos)
		}
		if newPos < 0 {
			break
		}
		outPos = newPos
	}

	// Extract this lead from interleaved buffer
	result := make([]int32, nSamples)
	for t := 0; t < nSamples; t++ {
		result[t] = output[t*8+leadIdx]
	}
	return result, nil
}

// s16trunc truncates an int to int16 range.
func s16trunc(v int) int32 {
	v = v & 0xFFFF
	if v >= 0x8000 {
		return int32(v - 0x10000)
	}
	return int32(v)
}

// mode0Write: simple write of count samples to output[t*8+lead].
// Returns new outPos or -1 on overflow.
func mode0Write(output []int32, maxEnd int, seg []int, count, lead, pos int) int {
	endPos := pos + count
	if endPos > maxEnd {
		return -1
	}
	for i := 0; i < count && i < len(seg); i++ {
		idx := (pos+i)*8 + lead
		if idx >= 0 && idx < len(output) {
			output[idx] = s16trunc(seg[i])
		}
	}
	return endPos
}

// mode1Upsample: 2× upsampling with midpoint interpolation.
// Returns new outPos or -1 on overflow.
func mode1Upsample(output []int32, maxEnd int, seg []int, count, lead, pos, param int) int {
	if count < 5 {
		endPos := pos + count
		if endPos > maxEnd {
			return -1
		}
		for i := 0; i < count && i < len(seg); i++ {
			idx := (pos+i)*8 + lead
			if idx >= 0 && idx < len(output) {
				output[idx] = s16trunc(seg[i])
			}
		}
		return endPos
	}

	inner := count - 4
	projected := pos + param + 3 + inner*2
	if projected > maxEnd {
		return -1
	}

	writeOut := func(t, l int, val int) {
		idx := t*8 + l
		if idx >= 0 && idx < len(output) {
			output[idx] = s16trunc(val)
		}
	}
	readOut := func(t, l int) int {
		idx := t*8 + l
		if idx >= 0 && idx < len(output) {
			return int(output[idx])
		}
		return 0
	}

	writeOut(pos, lead, seg[0])

	uVar5 := pos + 2
	iVar4 := 1
	sVar1 := 1

	if 1 < inner {
		for sVar1 < inner {
			writeOut(uVar5, lead, seg[iVar4])
			midpoint := (readOut(uVar5-2, lead) + readOut(uVar5, lead)) / 2
			writeOut(uVar5-1, lead, midpoint)
			uVar5 += 2
			sVar1++
			iVar4 = sVar1
		}
	}

	if param == 0 {
		uVar5--
	}

	for j := 0; j < 4; j++ {
		if sVar1+j < len(seg) {
			writeOut(uVar5+j, lead, seg[sVar1+j])
		}
	}

	if param == 1 {
		v := (readOut(uVar5-2, lead) + readOut(uVar5, lead)) / 2
		writeOut(uVar5-1, lead, v)
	}

	return uVar5 + 4
}

package nktofda

import (
	"bytes"
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

// frameLayout holds the per-lead frame layout derived from the RECORD section's
// frame descriptor. All offsets are relative to the RECORD section data start.
type frameLayout struct {
	flags    int   // gap-frame flag header (mode/param bits) start
	gapCtOff int   // code-table offset of the Gap frame (Lead I)
	frames   []int // frame_start offsets for leads II, V1..V6 (7 entries)
	nSeg     int
	modes    []int
	params   []int
}

// deriveFrameLayout locates the rhythm frames generically.
//
// Earlier code scanned a fixed byte range for an "offset table" and fell back to
// constants from 00000005.DAT. That table does not exist: those constants are
// specific to one recording, and the scan instead matched the beat/R-peak table
// (small ascending values), producing garbage frame offsets and all-zero leads.
//
// The robust scheme uses the frame descriptor (flag 0xFF00 immediately followed
// by total_samples) which every recording carries:
//
//	descriptor+0x00  0xFF00
//	descriptor+0x02  total_samples
//	descriptor+0x04  u16 -> (descriptor+4)+that = gap flag header ("flags")
//	flags+0x00       u16 frame size: frameII = flags + this
//	flags+0x02       u16 bit_count; n_segments = bit_count>>2
//	flags+0x04..     packed mode/param bytes, ceil(n_seg/2) padded to even
//	<after flags hdr> Gap frame code table (Lead I)
//
// Leads II..V6 are a linked list: each frame_start holds, at +0x00, the byte size
// of that frame, so the next frame begins at frame_start + u16(frame_start).
func deriveFrameLayout(rec []byte, totalSamples int) (*frameLayout, error) {
	sig := []byte{0xFF, 0x00, byte(totalSamples >> 8), byte(totalSamples)}
	d := bytes.Index(rec, sig)
	if d < 0 {
		return nil, fmt.Errorf("frame descriptor (0xFF00 + total_samples=%d) not found", totalSamples)
	}
	if d+6 > len(rec) {
		return nil, fmt.Errorf("frame descriptor truncated")
	}
	flags := d + 4 + int(binary.BigEndian.Uint16(rec[d+4:d+6]))
	if flags+4 > len(rec) {
		return nil, fmt.Errorf("flag header out of bounds")
	}
	nSeg, modes, params, err := extractModesParams(rec, flags)
	if err != nil {
		return nil, err
	}

	// Gap (Lead I) code table follows the variable-length flag header.
	flagBytes := (nSeg + 1) / 2
	if flagBytes&1 != 0 {
		flagBytes++ // pad to a 16-bit word boundary
	}
	gapCtOff := flags + 4 + flagBytes

	// Leads II..V6: chained via the per-frame size word at frame_start+0x00.
	frames := make([]int, 0, 7)
	fs := flags + int(binary.BigEndian.Uint16(rec[flags:flags+2])) // frame II
	for i := 0; i < 7; i++ {
		if fs+2 > len(rec) {
			return nil, fmt.Errorf("frame %d start 0x%x out of bounds", i+1, fs)
		}
		frames = append(frames, fs)
		fs += int(binary.BigEndian.Uint16(rec[fs : fs+2]))
	}

	return &frameLayout{flags: flags, gapCtOff: gapCtOff, frames: frames, nSeg: nSeg, modes: modes, params: params}, nil
}

// DecodeLeads decodes all 8 measured leads from the RECORD section data.
// rec = RECORD section data (starts at section_offset + 14 in the file).
// nSamples = total samples per lead (e.g. 5000).
// avg holds the median-beat templates used to reconstruct QRS zones (segment
// modes 2/3); it may be nil for recordings that use only modes 0/1.
func DecodeLeads(rec []byte, nSamples int, avg *avgTemplates) (map[string][]int32, error) {
	if len(rec) < 4 {
		return nil, fmt.Errorf("RECORD data too short")
	}
	compType := int(binary.BigEndian.Uint16(rec[4:6]))
	if compType != 16 {
		return nil, fmt.Errorf("unsupported compression type %d (only type 16 supported)", compType)
	}

	// Locate the frames generically from the frame descriptor.
	layout, err := deriveFrameLayout(rec, nSamples)
	if err != nil {
		return nil, fmt.Errorf("deriving frame layout: %w", err)
	}

	// Frame definitions: (leadName, ct_off, lead_index). Lead I (Gap) uses the
	// code-table offset after the flag header; leads II..V6 use frame_start+2.
	type frameSpec struct {
		name    string
		ctOff   int
		leadIdx int
	}
	frames := []frameSpec{{name: "I", ctOff: layout.gapCtOff, leadIdx: 0}}
	leadNames := []string{"II", "V1", "V2", "V3", "V4", "V5", "V6"}
	for i, fs := range layout.frames {
		frames = append(frames, frameSpec{name: leadNames[i], ctOff: fs + 2, leadIdx: i + 1})
	}

	result := make(map[string][]int32, 8)
	for _, fr := range frames {
		if fr.ctOff+2 > len(rec) {
			return nil, fmt.Errorf("lead %s code table out of bounds", fr.name)
		}
		ctSize := int(binary.BigEndian.Uint16(rec[fr.ctOff : fr.ctOff+2]))
		dataOff := fr.ctOff + 2 + ctSize
		samples, err := decodeLeadFrame(rec, fr.ctOff, dataOff, layout.nSeg, layout.modes, layout.params, fr.leadIdx, nSamples, avg)
		if err != nil {
			return nil, fmt.Errorf("decoding lead %s: %w", fr.name, err)
		}
		result[fr.name] = samples
	}
	return result, nil
}

// extractModesParams extracts n_segments, modes[], params[] from the Gap frame
// flag header. The mode/param bits are packed into u16 words starting at
// flags+4; the number of words depends on n_segments, so we read as many as the
// last segment indexes (the original fixed 7-word read truncated files with
// n_segments > ~28, e.g. n_segments=33).
func extractModesParams(rec []byte, sectionStart int) (int, []int, []int, error) {
	if sectionStart+4 > len(rec) {
		return 0, nil, nil, fmt.Errorf("Gap frame header out of bounds")
	}
	bitCount := int(binary.BigEndian.Uint16(rec[sectionStart+2 : sectionStart+4]))
	nSegments := bitCount >> 2
	if nSegments <= 0 || nSegments > 1024 {
		return 0, nil, nil, fmt.Errorf("implausible n_segments %d", nSegments)
	}

	maxWordIdx := ((nSegments - 1) >> 2) + 2 // word index touched by the last segment
	au := make([]int, maxWordIdx+5)          // +slack so clamping below is a no-op
	for i := 2; i <= maxWordIdx; i++ {
		off := sectionStart + i*2
		if off+2 <= len(rec) {
			au[i] = int(binary.BigEndian.Uint16(rec[off : off+2]))
		}
	}

	modes := make([]int, nSegments)
	params := make([]int, nSegments)
	for i := 0; i < nSegments; i++ {
		uVar8 := i * 4
		wordIdx := (uVar8 >> 4) + 2
		if wordIdx >= len(au) {
			wordIdx = len(au) - 1
		}
		uVar14 := au[wordIdx]
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
func decodeLeadFrame(rec []byte, ctOff, dataStart, nSegments int, modes, params []int, leadIdx, nSamples int, avg *avgTemplates) ([]int32, error) {
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
		switch {
		case mode == 0:
			newPos = mode0Write(output, nSamples, segVals, nChunk, leadIdx, outPos)
		case mode == 1:
			newPos = mode1Upsample(output, nSamples, segVals, nChunk, leadIdx, outPos, param)
		case mode >= 2 && avg != nil && leadIdx < len(avg.tpl) && avg.tpl[leadIdx] != nil:
			// QRS subtraction zone: re-add the median beat. Mode 2 begins the
			// reconstruction at the pre-QRS offset, mode 3 at the zone midpoint.
			subOff := avg.subOffset
			if mode == 3 {
				subOff = avg.zoneMid
			}
			newPos = mode23QRSZone(output, nSamples, segVals, nChunk, avg.tpl[leadIdx],
				leadIdx, outPos, avg.zoneMid, avg.zoneEnd, param, subOff)
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

// mode23QRSZone reconstructs a segment that spans a QRS subtraction zone.
//
// The compressed stream only carries the inter-beat residual around each QRS;
// the median beat (avg) is re-added here. The zone has three boundaries on the
// median template: subOffset (where the pre-QRS reconstruction starts), zoneMid
// (the QRS midpoint) and zoneEnd. Reconstruction proceeds in three parts:
//
//  1. Pre-zone: add the median at stride 2 up to zoneMid, then 2× upsample.
//  2. QRS core (zoneMid..zoneEnd): write residual+median directly to the output.
//  3. Post-zone: add the median at stride 2 for the remainder, then 2× upsample.
//
// Returns the new output position, or -1 on overflow.
func mode23QRSZone(output []int32, maxEnd int, segment []int, count int, avg []int32,
	lead, pos, zoneMid, zoneEnd, param, subOffset int) int {

	seg := make([]int, count)
	copy(seg, segment)

	avgAt := func(i int) int {
		if i >= 0 && i < len(avg) {
			return int(avg[i])
		}
		return 0
	}
	writeOut := func(t, val int) {
		idx := t*8 + lead
		if idx >= 0 && idx < len(output) {
			output[idx] = s16trunc(val)
		}
	}

	uVar9 := 0

	// Part 1: pre-zone median addition (stride 2) then 2× upsample.
	if subOffset < zoneMid {
		cVar2 := (zoneMid - subOffset) & 0xFF
		idx := subOffset
		for idx < zoneMid-4 && uVar9 < count {
			subOffset += 2
			seg[uVar9] = int(s16trunc(seg[uVar9] + avgAt(idx)))
			uVar9++
			idx = subOffset
		}
		for cur := zoneMid - 4; cur < zoneMid && uVar9 < count; cur++ {
			seg[uVar9] = int(s16trunc(seg[uVar9] + avgAt(cur)))
			uVar9++
		}
		newPos := mode1Upsample(output, maxEnd, seg[:uVar9], uVar9, lead, pos, (cVar2-1)&1)
		if newPos < 0 {
			return -1
		}
		pos = newPos
	}

	remaining := count - uVar9
	if remaining <= 0 {
		return pos
	}

	// Determine the QRS-core span written directly to the output.
	var sVar6 int
	if zoneEnd-zoneMid < remaining {
		sVar6 = zoneEnd + 1
	} else {
		sVar6 = (zoneMid - uVar9) + count
	}
	if pos-zoneMid+sVar6 > maxEnd {
		return -1
	}

	avgIdx := zoneMid
	// Part 2: QRS core — direct output write of residual + median.
	if zoneMid < sVar6 {
		avgPos := zoneMid
		for n := sVar6 - zoneMid; n > 0 && uVar9 < count; n-- {
			writeOut(pos, seg[uVar9]+avgAt(avgPos))
			pos++
			uVar9++
			avgPos++
		}
		avgIdx = avgPos
	}

	// Part 3: post-zone median addition (stride 2, then stride 1 for the tail).
	segIdx := uVar9
	for segIdx < count-4 {
		seg[segIdx] = int(s16trunc(seg[segIdx] + avgAt(avgIdx)))
		avgIdx += 2
		segIdx++
	}
	if param == 0 {
		avgIdx--
	}
	for segIdx < count {
		seg[segIdx] = int(s16trunc(seg[segIdx] + avgAt(avgIdx)))
		avgIdx++
		segIdx++
	}

	postCount := count - uVar9
	if postCount > 0 {
		newPos := mode1Upsample(output, maxEnd, seg[uVar9:], postCount, lead, pos, param)
		if newPos >= 0 {
			pos = newPos
		}
	}
	return pos
}

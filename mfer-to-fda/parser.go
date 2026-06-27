package mfertofda

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// MFER frame tags used by this parser (subset of ISO/TS 11073-92001 relevant to
// the NK 12-lead export). All offsets/semantics confirmed against a reference
// .mwf + its FDA aECG ground truth.
const (
	tagPreamble     = 0x40 // "MFR Standard 12 leads ECG"
	tagManufacturer = 0x17 // "NIHON KOHDEN^2350K^03.04"
	tagUID          = 0x87 // instance UID string
	tagComment      = 0x45 // free-text comment (source path)
	tagByteOrder    = 0x01 // 0 = big-endian, 1 = little-endian
	tagDatetime     = 0x85 // [year u16, month, day, hour, min, sec]
	tagInfo         = 0x11 // "KEY=VALUE" strings (filters, …)
	tagEvent        = 0x16 // event label ("REST-ECG")
	tagNumSamples   = 0x04 // samples per channel for the following data block
	tagNumChannels  = 0x05 // number of stored channels (leads)
	tagInterval     = 0x0b // sampling interval [unit, exp(int8), mantissa]
	tagWaveform     = 0x1e // raw waveform data (planar int16)
	tagGroupEnd     = 0x00 // 1-byte group terminator / pad
)

// storedLeadIndex maps the 8 leads physically stored by NK MFER to their
// position in the 12-lead output order. III, aVR, aVL, aVF are derived.
const (
	idxI = iota
	idxII
	idxIII
	idxAVR
	idxAVL
	idxAVF
	idxV1
	idxV2
	idxV3
	idxV4
	idxV5
	idxV6
)

// storedOrder is the lead order of the 8 channels inside each waveform block.
var storedOrder = [8]int{idxI, idxII, idxV1, idxV2, idxV3, idxV4, idxV5, idxV6}

// ParseFile parses an MFER (.mwf) byte slice into MferData.
func ParseFile(dat []byte) (*MferData, error) {
	if len(dat) < 2 || dat[0] != tagPreamble {
		return nil, fmt.Errorf("not an MFER file (missing 0x40 preamble)")
	}

	d := &MferData{Scale: 1.25, NumChannels: 8}
	littleEndian := true // overridden by the byte-order frame if present

	// Per-block state carried forward as control frames are encountered.
	curChannels := 8
	blockIdx := 0

	i := 0
	for i < len(dat) {
		tag := dat[i]
		i++
		if tag == tagGroupEnd {
			continue // standalone terminator/pad, no length
		}
		if i >= len(dat) {
			break
		}
		length, ni, ok := readLen(dat, i)
		if !ok {
			break
		}
		i = ni
		if i+length > len(dat) {
			break
		}
		val := dat[i : i+length]
		i += length

		switch tag {
		case tagByteOrder:
			if length >= 1 {
				littleEndian = val[0] == 1
			}
		case tagManufacturer:
			man, model, ver := splitDeviceField(string(val))
			d.Manufacturer = normalizeManufacturer(man)
			d.ModelName = model
			d.SoftwareVer = ver
		case tagDatetime:
			if date, tm, ok := mferTime(val, littleEndian); ok {
				d.StudyDate, d.StudyTime = date, tm
			}
		case tagInfo:
			applyInfo(d, string(val))
		case tagNumChannels:
			if n := leInt(val, littleEndian); n > 0 {
				curChannels = n
				d.NumChannels = n
			}
		case tagInterval:
			if r := samplingRate(val, littleEndian); r > 0 {
				d.SampleRate = r
			}
		case tagWaveform:
			leads := decodeWaveform(val, curChannels, littleEndian)
			deriveLimbLeads(&leads)
			switch blockIdx {
			case 0:
				d.RhythmLeads = leads
			case 1:
				d.MedianLeads = leads
			}
			blockIdx++
		}
	}

	if d.SampleRate == 0 {
		d.SampleRate = 500 // NK MFER default
	}
	return d, nil
}

// readLen decodes an MFER length field (BER-style): a single byte L; if L < 0x80
// it is the length, otherwise its low 7 bits give the number of following
// big-endian length bytes.
func readLen(buf []byte, i int) (length, next int, ok bool) {
	if i >= len(buf) {
		return 0, i, false
	}
	L := int(buf[i])
	i++
	if L < 0x80 {
		return L, i, true
	}
	n := L & 0x7F
	if i+n > len(buf) {
		return 0, i, false
	}
	v := 0
	for k := 0; k < n; k++ {
		v = (v << 8) | int(buf[i+k])
	}
	return v, i + n, true
}

// leInt reads a little/big-endian unsigned integer from a small value field.
func leInt(b []byte, littleEndian bool) int {
	if len(b) == 0 {
		return 0
	}
	v := 0
	if littleEndian {
		for k := len(b) - 1; k >= 0; k-- {
			v = (v << 8) | int(b[k])
		}
	} else {
		for k := 0; k < len(b); k++ {
			v = (v << 8) | int(b[k])
		}
	}
	return v
}

// samplingRate decodes the MFER sampling-interval frame and returns the
// frequency in Hz. Value layout: [unit code][exponent int8][mantissa int LE].
// E.g. 01 FD 02 00 → unit=s, exp=-3, mantissa=2 → 2e-3 s → 500 Hz.
func samplingRate(b []byte, littleEndian bool) float64 {
	if len(b) < 3 {
		return 0
	}
	exp := int(int8(b[1]))
	mant := leInt(b[2:], littleEndian)
	if mant == 0 {
		return 0
	}
	interval := float64(mant)
	for e := 0; e < exp; e++ {
		interval *= 10
	}
	for e := 0; e > exp; e-- {
		interval /= 10
	}
	if interval <= 0 {
		return 0
	}
	return 1.0 / interval
}

// decodeWaveform decodes a planar int16 waveform block into a 12-lead array.
// Layout: [channel0: nSamples][channel1: nSamples]…, int16 (LE/BE per header).
// The per-channel sample count is derived from the block length (block-length /
// nChannels / 2) — robust, since MFER's 0x04 sample-count tag is overloaded with
// channel-attribute frames. Derived leads are left empty (see deriveLimbLeads).
func decodeWaveform(b []byte, nChannels int, littleEndian bool) [12][]int16 {
	var leads [12][]int16
	if nChannels <= 0 || nChannels > 32 {
		nChannels = 8
	}
	nSamples := len(b) / 2 / nChannels
	for ch := 0; ch < nChannels && ch < len(storedOrder); ch++ {
		samples := make([]int16, 0, nSamples)
		base := ch * nSamples * 2
		for s := 0; s < nSamples; s++ {
			off := base + s*2
			if off+2 > len(b) {
				break
			}
			var v int16
			if littleEndian {
				v = int16(binary.LittleEndian.Uint16(b[off : off+2]))
			} else {
				v = int16(binary.BigEndian.Uint16(b[off : off+2]))
			}
			samples = append(samples, v)
		}
		leads[storedOrder[ch]] = samples
	}
	return leads
}

// deriveLimbLeads computes III, aVR, aVL and aVF from leads I and II
// (Einthoven / Goldberger relations).
func deriveLimbLeads(leads *[12][]int16) {
	i, ii := leads[idxI], leads[idxII]
	n := min(len(i), len(ii))
	if n == 0 {
		return
	}
	iii := make([]int16, n)
	avr := make([]int16, n)
	avl := make([]int16, n)
	avf := make([]int16, n)
	for k := 0; k < n; k++ {
		a, b := int(i[k]), int(ii[k])
		iii[k] = int16(b - a)
		avr[k] = int16(-(a + b) / 2)
		avl[k] = int16(a - b/2)
		avf[k] = int16(b - a/2)
	}
	leads[idxIII] = iii
	leads[idxAVR] = avr
	leads[idxAVL] = avl
	leads[idxAVF] = avf
}

// applyInfo parses a "KEY=VALUE" info frame and records known acquisition
// settings (filters). Unknown keys are ignored.
func applyInfo(d *MferData, s string) {
	s = strings.TrimSpace(s)
	key, val, ok := strings.Cut(s, "=")
	if !ok {
		return
	}
	switch strings.ToUpper(strings.TrimSpace(key)) {
	case "HPF": // high-pass filter, e.g. "0.04"
		d.FilterHPF = atof(val)
	case "BEF": // band-elimination (hum/AC) filter, e.g. "50^Hum filter"
		num, _, _ := strings.Cut(val, "^")
		d.NotchFilter = atof(num)
	}
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

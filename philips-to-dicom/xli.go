package philipstodicom

// XLI/LZW waveform decoder for Philips SierraECG format.
// Ported from gosierraecg (github.com/...); all logic preserved.

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
)

const (
	sierraECGSamples = 5500
	lzwBits          = 10
	lzwMaxValue      = (1 << lzwBits) - 1
	lzwMaxCode       = lzwMaxValue - 1
	lzwTableSize     = 5021
)

// decodeRhythmLeads decodes the XLI-compressed parsedwaveforms block into 12 leads.
func decodeRhythmLeads(pw ParsedWaveforms) ([12][]int16, error) {
	if !strings.EqualFold(pw.DataEncoding, "Base64") {
		return [12][]int16{}, fmt.Errorf("unsupported data encoding: %q", pw.DataEncoding)
	}

	decoded, err := base64.StdEncoding.DecodeString(pw.Data)
	if err != nil {
		return [12][]int16{}, fmt.Errorf("base64 decode: %w", err)
	}

	if len(decoded) == 0 {
		return [12][]int16{}, fmt.Errorf("empty waveform data")
	}

	var leads [12][]int16
	leadNames := []string{"I", "II", "III", "aVR", "aVL", "aVF", "V1", "V2", "V3", "V4", "V5", "V6"}
	for i := range 12 {
		leads[i] = make([]int16, sierraECGSamples)
		_ = leadNames[i]
	}

	offset := 0
	for lead := 0; lead < 12 && offset < len(decoded); lead++ {
		n, err := xliDecodeChunk(decoded[offset:], leads[lead], sierraECGSamples)
		if err != nil {
			return [12][]int16{}, fmt.Errorf("decode lead %d: %w", lead, err)
		}
		offset += n
	}

	// Reconstruct derived leads
	leadI := leads[0]
	leadII := leads[1]
	leadIII := leads[2]
	leadAVR := leads[3]
	leadAVL := leads[4]
	leadAVF := leads[5]

	for j := range sierraECGSamples {
		leads[2][j] = leadII[j] - leadI[j] - leadIII[j]
		leads[3][j] = -((leadI[j] + leadII[j]) / 2) - leadAVR[j]
		leads[4][j] = ((leadI[j] - leadIII[j]) / 2) - leadAVL[j]
		leads[5][j] = ((leadII[j] + leadIII[j]) / 2) - leadAVF[j]
	}

	return leads, nil
}

// xliDecodeChunk decompresses one XLI chunk; returns bytes consumed.
func xliDecodeChunk(chunk []byte, samples []int16, count int) (int, error) {
	if len(chunk) < 8 {
		return 0, fmt.Errorf("chunk too small")
	}
	size := binary.LittleEndian.Uint32(chunk[0:4])
	lastValue := int16(binary.LittleEndian.Uint16(chunk[6:8]))

	ctx := newLZWContext(chunk[8 : 8+size])
	deltas := make([]byte, count*2)
	if err := ctx.expand(deltas); err != nil {
		return 0, fmt.Errorf("LZW expand: %w", err)
	}

	for j := range count {
		msb := deltas[j]
		lsb := deltas[count+j]
		samples[j] = int16(msb)<<8 | int16(lsb)
	}

	if count >= 2 {
		x := samples[0]
		y := samples[1]
		for j := 2; j < count; j++ {
			z := (y + y) - x - lastValue
			lastValue = samples[j] - 64
			samples[j] = z
			x = y
			y = z
		}
	}

	return int(size) + 8, nil
}

// lzwContext is a simple LZW decompressor.
type lzwContext struct {
	decodeStack     [4000]byte
	prefixCode      []uint32
	appendCharacter []byte
	input           []byte
	inputLength     int
	pos             int
	inputBitCount   int
	inputBitBuffer  uint32
}

func newLZWContext(input []byte) *lzwContext {
	return &lzwContext{
		input:           input,
		inputLength:     len(input),
		prefixCode:      make([]uint32, lzwTableSize),
		appendCharacter: make([]byte, lzwTableSize),
	}
}

func (ctx *lzwContext) inputCode() uint32 {
	for ctx.inputBitCount <= 24 {
		if ctx.pos >= ctx.inputLength {
			break
		}
		ctx.inputBitBuffer |= uint32(ctx.input[ctx.pos]) << (24 - ctx.inputBitCount)
		ctx.pos++
		ctx.inputBitCount += 8
	}
	v := ctx.inputBitBuffer >> (32 - lzwBits)
	ctx.inputBitBuffer <<= lzwBits
	ctx.inputBitCount -= lzwBits
	return v & 0x0000FFFF
}

func (ctx *lzwContext) decodeString(stackOffset int, code uint32) ([]byte, byte) {
	i := 0
	for code > 255 {
		ctx.decodeStack[stackOffset] = ctx.appendCharacter[code]
		stackOffset++
		code = ctx.prefixCode[code]
		i++
		if i >= lzwMaxCode {
			return nil, 0
		}
	}
	ctx.decodeStack[stackOffset] = byte(code)
	return ctx.decodeStack[:stackOffset+1], byte(code)
}

func (ctx *lzwContext) expand(output []byte) error {
	nextCode := uint32(256)
	oldCode := ctx.inputCode()
	character := byte(oldCode)
	outputPos := 0
	output[outputPos] = character
	outputPos++

	for {
		newCode := ctx.inputCode()
		if newCode == lzwMaxValue {
			break
		}
		var str []byte
		if newCode >= nextCode {
			ctx.decodeStack[0] = character
			str, character = ctx.decodeString(1, oldCode)
		} else {
			str, character = ctx.decodeString(0, newCode)
		}
		for i := len(str) - 1; i >= 0; i-- {
			if outputPos >= len(output) {
				return fmt.Errorf("output buffer overflow")
			}
			output[outputPos] = str[i]
			outputPos++
		}
		if nextCode <= lzwMaxCode {
			ctx.prefixCode[nextCode] = oldCode
			ctx.appendCharacter[nextCode] = character
			nextCode++
		}
		oldCode = newCode
	}
	return nil
}

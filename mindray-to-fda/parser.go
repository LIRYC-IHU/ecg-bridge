package mindraytofda

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

var magic = []byte{0xEB, 0xEC, 0x12, 0x34, 0x20}

func ParseFile(dat []byte) (*MindrayData, error) {
	if len(dat) < 0xAF21 {
		return nil, fmt.Errorf("file too small: %d bytes", len(dat))
	}
	if !matchMagic(dat) {
		return nil, fmt.Errorf("invalid magic: expected EB EC 12 34 20")
	}

	md := &MindrayData{}
	md.Patient = parsePatient(dat)
	md.Device = parseDevice(dat)
	md.Record = parseRecord(dat)
	md.Measurement = parseObservations(dat)
	md.Leads = parseLeads(dat)
	return md, nil
}

func matchMagic(dat []byte) bool {
	if len(dat) < len(magic) {
		return false
	}
	for i, b := range magic {
		if dat[i] != b {
			return false
		}
	}
	return true
}

func parsePatient(dat []byte) PatientData {
	pd := PatientData{}

	pd.Name = readNullStr(dat, 0x277, 48)
	pd.PatientID = readNullStr(dat, 0x24d, 20)

	genderCode := dat[0x2A9]
	switch genderCode {
	case 1:
		pd.Gender = "M"
	case 2:
		pd.Gender = "F"
	default:
		pd.Gender = "UN"
	}

	pd.Paced = (dat[0x2A5] & 0x01) != 0
	pd.Location = readNullStr(dat, 0x2bf, 40)

	// Timestamps (unix BE)
	if len(dat) > 0x54 {
		lowTS := binary.BigEndian.Uint32(dat[0x50:0x54])
		if lowTS > 0 {
			pd.StartTime = time.Unix(int64(lowTS), 0).UTC()
		}
	}
	if len(dat) > 0x20 {
		highTS := binary.BigEndian.Uint32(dat[0x1c:0x20])
		if highTS > 0 {
			pd.EndTime = time.Unix(int64(highTS), 0).UTC()
		}
	}

	return pd
}

func parseDevice(dat []byte) DeviceData {
	dd := DeviceData{
		ModelName:    "BeneHeart R12",
		Manufacturer: "(C) Shenzhen Mindray Bio-Medical Electronics Co., Ltd. All rights reserved.",
	}
	dd.SoftwareName = readNullStr(dat, 0x6c, 16)
	dd.SerialNumber = readNullStr(dat, 0x7c, 20)
	return dd
}

const observationOffset = 0x350

func parseObservations(dat []byte) MeasurementData {
	md := MeasurementData{}
	if len(dat) < observationOffset+92 {
		return md
	}
	d := dat[observationOffset:]

	read32 := func(off int) int {
		return int(int32(binary.BigEndian.Uint32(d[off : off+4])))
	}

	// Layout: [none(4)] [units(4) value(4)] x5 then RV5/SV1 then axes
	md.HeartRate = read32(8)
	md.PRInterval = read32(16)
	md.QRSDuration = read32(24)
	md.QTInterval = read32(32)
	md.QTcInterval = read32(40)

	// P axis at offset 64, QRS axis at 72, T axis at 80 (each preceded by unit field)
	pAxis := read32(64)
	qrsAxis := read32(72)
	tAxis := read32(80)

	if pAxis != -100 {
		md.PAxis = pAxis
		md.HasPAxis = true
	}
	if qrsAxis != -100 {
		md.QRSAxis = qrsAxis
		md.HasQRSAxis = true
	}
	if tAxis != -100 {
		md.TAxis = tAxis
		md.HasTAxis = true
	}

	return md
}

func parseRecord(dat []byte) RecordParams {
	rp := RecordParams{
		Scale: 1.0,
	}
	// Sampling rate from lead_info_block at 0xac58
	// Each block is 60 bytes: 16(name) + 10(pad) + 2(sr BE) + 4(scale) + 16 + 12
	if len(dat) > 0xac58+28 {
		rp.SampleRate = int(binary.BigEndian.Uint16(dat[0xac58+26 : 0xac58+28]))
	}
	if rp.SampleRate == 0 {
		rp.SampleRate = 1000
	}
	return rp
}

type leadDef struct {
	name   string
	offset int
}

var leadDefs = []leadDef{
	{"I", 0xaf21},
	{"II", 0x14b61},
	{"III", 0x1e7a1},
	{"aVR", 0x283e1},
	{"aVL", 0x32021},
	{"aVF", 0x3bc61},
	{"V1", 0x458a1},
	{"V2", 0x4f4e1},
	{"V3", 0x59121},
	{"V4", 0x62d61},
	{"V5", 0x6c9a1},
	{"V6", 0x765e1},
}

func parseLeads(dat []byte) map[string][]int {
	leads := make(map[string][]int, 12)
	for _, ld := range leadDefs {
		samples := decodeLead(dat, ld.offset)
		if len(samples) > 0 {
			leads[ld.name] = samples
		}
	}
	return leads
}

// decodeLead decodes Mindray waveform data.
// Each sample is 4 bytes: [s0, s1, value_raw, skip]
// s1=0x80 → value is positive (value_raw as unsigned)
// s1=0x7F → value is negative (value_raw - 256)
// s1=0x7E → value is deeply negative (value_raw - 512)
// Terminator: s0=0xFF, s1=0xFF
func decodeLead(dat []byte, offset int) []int {
	const maxSamples = 10000
	const sampleSize = 4

	if offset+maxSamples*sampleSize > len(dat) {
		return nil
	}

	samples := make([]int, 0, maxSamples)
	for i := 0; i < maxSamples; i++ {
		pos := offset + i*sampleSize
		s1 := dat[pos+1]
		valueRaw := int(dat[pos+2])

		if dat[pos] == 0xFF && s1 == 0xFF {
			break
		}

		var value int
		switch s1 {
		case 0x80:
			value = valueRaw
		case 0x7F:
			value = valueRaw - 256
		case 0x7E:
			value = valueRaw - 512
		default:
			value = valueRaw
		}
		samples = append(samples, value)
	}
	return samples
}

func readNullStr(dat []byte, offset, maxLen int) string {
	if offset+maxLen > len(dat) {
		maxLen = len(dat) - offset
	}
	b := dat[offset : offset+maxLen]
	end := len(b)
	for i, c := range b {
		if c == 0 {
			end = i
			break
		}
	}
	return strings.TrimSpace(string(b[:end]))
}

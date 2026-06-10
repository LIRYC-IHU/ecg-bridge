package nktofda

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

const pecHeaderSize = 14 // 10B preamble + 2B SIZE + 2B TYPE

// pec section type IDs
const (
	secPointer        = 0x0000
	secSystem         = 0x0001
	secPatient        = 0x0002
	secCriteria       = 0x0006
	secMeasurement    = 0x0007
	secRecord         = 0x0008
	sec12Lead         = 0x0100
	secSJISPatient    = 0x0103
	secAnaCondition   = 0x0104
	secRecExtend      = 0x0108
	secAnalysis       = 0x0110
	secPatient2       = 0x0113
	secRecExtendAdd   = 0x0115
	secExerciseLoad   = 0x0200
	secNehbRecord     = 0x0120
)

// section holds the offset and data of a PEC section.
type section struct {
	offset uint32
	data   []byte
}

// parseSections scans the file and builds a map of section type → section.
func parseSections(dat []byte) (map[uint16]section, error) {
	sections := make(map[uint16]section)
	off := 0
	for off+pecHeaderSize < len(dat) {
		if off+14 > len(dat) {
			break
		}
		size := int(binary.BigEndian.Uint16(dat[off+10 : off+12]))
		secType := binary.BigEndian.Uint16(dat[off+12 : off+14])
		if size < pecHeaderSize {
			break
		}
		dataEnd := off + size
		if dataEnd > len(dat) {
			break
		}
		dataSlice := dat[off+pecHeaderSize : dataEnd]
		sections[secType] = section{
			offset: uint32(off),
			data:   dataSlice,
		}
		// advance past section + 2B CRC16
		off = dataEnd + 2
	}
	if _, ok := sections[secRecord]; !ok {
		// Nehb-lead recordings carry their waveform in section 0x0120 with a
		// different layout (no RECORD/MEASUREMENT sections, no frame descriptor)
		// and are not yet supported by this decoder.
		if _, nehb := sections[secNehbRecord]; nehb {
			return nil, fmt.Errorf("unsupported recording type: Nehb-lead recording (waveform in section 0x0120); standard 12-lead RECORD section (0x0008) not present")
		}
		return nil, fmt.Errorf("RECORD section (0x0008) not found")
	}
	return sections, nil
}

// ParseFile parses a NK .DAT file and returns NKData without waveforms.
func ParseFile(dat []byte) (*NKData, error) {
	secs, err := parseSections(dat)
	if err != nil {
		return nil, err
	}

	nd := &NKData{}
	nd.Patient = parsePatient(secs)
	nd.Measurement = parseMeasurement(secs)
	nd.Record = parseRecord(secs)
	return nd, nil
}

// parsePatient extracts demographic data.
func parsePatient(secs map[uint16]section) PatientData {
	pd := PatientData{}

	if s, ok := secs[secSystem]; ok {
		pd.DeviceModel = extractDeviceModel(s.data)
	}

	if s, ok := secs[secPatient]; ok {
		d := s.data
		if len(d) >= 0x173 {
			pd.FamilyName = readNullStr(d[0x00:0x20])
			pd.GivenName = readNullStr(d[0x20:0x3E])
			pd.PatientID = readSpaceStr(d[0x3E : 0x3E+9])
			if len(d) >= 0x160 {
				pd.Location = readNullStr(d[0x143:0x160])
			}
			if len(d) >= 0x173 {
				pd.RecordingAt = parseDatetime(d[0x16C:0x173])
			}
		}
	}

	if s, ok := secs[secPatient2]; ok {
		pd.Gender = parseGender(s.data)
	}

	return pd
}

// extractDeviceModel extracts model from SYSTEM data (e.g. "01002350K" → "2350K").
// NK models follow the pattern ddddK where d=digit. The binary often prefixes
// this with a manufacturer/series code (e.g. "0100"), so we scan for ddddK and
// return from that point.
func extractDeviceModel(data []byte) string {
	s := readNullStr(data)
	s = strings.TrimSpace(s)
	// Scan for pattern: 4 digits + 'K'
	for i := 0; i+4 < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' &&
			s[i+1] >= '0' && s[i+1] <= '9' &&
			s[i+2] >= '0' && s[i+2] <= '9' &&
			s[i+3] >= '0' && s[i+3] <= '9' &&
			i+4 < len(s) && s[i+4] == 'K' {
			// Extract model (ddddK) and trim any trailing spaces/padding
			end := i + 5 // position after 'K'
			return strings.TrimSpace(s[i:end])
		}
	}
	return s
}

// parseGender searches PATIENT2 data for the pattern 0x02 0x01 <gender_byte>.
func parseGender(data []byte) string {
	for i := 0; i+2 < len(data); i++ {
		if data[i] == 0x02 && data[i+1] == 0x01 {
			switch data[i+2] {
			case 0x01:
				return "M"
			case 0x02:
				return "F"
			case 0x03:
				return "U"
			}
		}
	}
	return ""
}

// parseMeasurement extracts ECG measurements from the MEASUREMENT section.
func parseMeasurement(secs map[uint16]section) MeasurementData {
	md := MeasurementData{}
	s, ok := secs[secMeasurement]
	if !ok || len(s.data) < 0x1C {
		return md
	}
	d := s.data

	md.HeartRate = readU16BE(d, 0x08)
	md.PRInterval = readU16BE(d, 0x0A)
	md.QRSDuration = readU16BE(d, 0x0C)
	md.QTInterval = readU16BE(d, 0x0E)
	md.QTcInterval = readU16BE(d, 0x10)

	pAxis := readI16BE(d, 0x12)
	qrsAxis := readI16BE(d, 0x14)
	tAxis := readI16BE(d, 0x16)

	// 0x8000 = not set sentinel
	if pAxis != -32768 {
		md.PAxis = pAxis
		md.HasPAxis = true
	}
	if qrsAxis != -32768 {
		md.QRSAxis = qrsAxis
		md.HasQRSAxis = true
	}
	if tAxis != -32768 {
		md.TAxis = tAxis
		md.HasTAxis = true
	}

	// Amplitudes stored in nV, convert to µV
	v5r := readU16BE(d, 0x18)
	v1s := readU16BE(d, 0x1A)
	if v5r != 0 && v5r != 0xFFFF {
		md.V5RAmplitude = float64(v5r) / 1000.0
	}
	if v1s != 0 && v1s != 0xFFFF {
		md.V1SAmplitude = float64(v1s) / 1000.0
	}

	return md
}

// parseRecord extracts waveform parameters from RECORD section.
//
// SampleRate comes from the RECORD header (+0x00, stable across files).
//
// TotalSamples is read from the 12LEAD section (0x0100) at +0x4E, which is at a
// fixed offset in every observed file. The RECORD section's own total_samples
// field (+0x1C4) is NOT reliable: it sits after a variable-length beat table,
// so its offset shifts per recording (observed 0x60, 0x64, 0x1C4) and reading a
// fixed +0x1C4 yields garbage (e.g. 62157, 2032) on files with a different beat
// count, which then corrupts the whole waveform decode.
func parseRecord(secs map[uint16]section) RecordParams {
	rp := RecordParams{Scale: 1.25}
	s, ok := secs[secRecord]
	if !ok || len(s.data) < 0x02 {
		return rp
	}
	rp.SampleRate = readU16BE(s.data, 0x00)

	// Preferred: num_samples from the 12LEAD section (fixed +0x4E offset).
	if l12, ok := secs[sec12Lead]; ok && len(l12.data) >= 0x50 {
		if n := readU16BE(l12.data, 0x4E); n > 0 {
			rp.TotalSamples = n
		}
	}
	// Fallback: RECORD +0x1C4 (only correct for files whose beat table ends there).
	if rp.TotalSamples == 0 && len(s.data) >= 0x1C6 {
		rp.TotalSamples = readU16BE(s.data, 0x1C4)
	}
	return rp
}

// parseDatetime decodes 7-byte NK datetime (year u16 BE, month, day, hour, min, sec).
func parseDatetime(b []byte) time.Time {
	if len(b) < 7 {
		return time.Time{}
	}
	year := int(binary.BigEndian.Uint16(b[0:2]))
	month := int(b[2])
	day := int(b[3])
	hour := int(b[4])
	min := int(b[5])
	sec := int(b[6])
	if year == 0 || month == 0 || day == 0 {
		return time.Time{}
	}
	return time.Date(year, time.Month(month), day, hour, min, sec, 0, time.UTC)
}

// readNullStr reads a null-terminated or null-padded ASCII string.
func readNullStr(b []byte) string {
	end := len(b)
	for i, c := range b {
		if c == 0 {
			end = i
			break
		}
	}
	return strings.TrimSpace(string(b[:end]))
}

// readSpaceStr reads space-terminated ASCII string.
func readSpaceStr(b []byte) string {
	for i, c := range b {
		if c == 0 || c == ' ' {
			return strings.TrimSpace(string(b[:i]))
		}
	}
	return strings.TrimSpace(string(b))
}

func readU16BE(d []byte, off int) int {
	if off+2 > len(d) {
		return 0
	}
	v := int(binary.BigEndian.Uint16(d[off : off+2]))
	if v == 0x8000 || v == 0xFFFF {
		return 0
	}
	return v
}

func readI16BE(d []byte, off int) int {
	if off+2 > len(d) {
		return -32768
	}
	raw := binary.BigEndian.Uint16(d[off : off+2])
	v := int(int16(raw))
	return v
}

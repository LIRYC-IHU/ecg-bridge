package fukudatofda

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// The file is a directory of units (layout read by nwHCOM.dll Nth_UntReadSell2,
// 0x1005f82c; every multi-byte field is big-endian):
//
//	0x1e      u16 unit count n
//	0x20      n entries of 16 bytes: u16 type, u16 sub, u32 size, 8 bytes
//	0x20+16n  the units, back to back, in entry order
//
// Each unit starts with its own u16 type, u16 sub, u32 size.
const (
	offUnitCount = 0x1e
	offUnitDir   = 0x20
	unitDirEntry = 16
	unitWave     = 9 // rhythm waveform unit
)

// Offsets inside the waveform unit (type 9), from the unit start.
const (
	waveInterval = 0x0a // u16 µs/sample
	waveScale    = 0x0c // u16 nV/digit
	waveSamples  = 0x0e // u32 samples per lead
	waveCompress = 0x12 // u16 0 = raw int16, otherwise Huffman
	waveLeads    = 0x14 // u16 lead count
	waveCodes    = 0x18 // u16 lead code per lead; then u32 data length, then data
	maxWaveLeads = 8    // >8 leads use other layouts in the DLL, not supported
)

// leadCodeName maps the on-file lead code to its name (the DLL stores only the
// 8 independent leads; III, aVR, aVL, aVF are derived).
var leadCodeName = map[uint16]string{
	1: "I", 2: "II", 3: "III", 4: "aVR", 5: "aVL", 6: "aVF",
	7: "V1", 8: "V2", 9: "V3", 10: "V4", 11: "V5", 12: "V6",
}

// findUnit returns the offset of the first unit of type typ.
func findUnit(dat []byte, typ uint16) (int, error) {
	if len(dat) < offUnitDir {
		return 0, fmt.Errorf("file too short for a unit directory")
	}
	n := int(binary.BigEndian.Uint16(dat[offUnitCount:]))
	off := offUnitDir + n*unitDirEntry
	if off > len(dat) {
		return 0, fmt.Errorf("unit directory (%d entries) runs past end of file", n)
	}
	for i := 0; i < n; i++ {
		e := dat[offUnitDir+i*unitDirEntry:]
		size := int(binary.BigEndian.Uint32(e[4:8]))
		if binary.BigEndian.Uint16(e[0:2]) == typ {
			if off+size > len(dat) {
				return 0, fmt.Errorf("unit type %d at %#x runs past end of file", typ, off)
			}
			return off, nil
		}
		off += size
	}
	return 0, fmt.Errorf("no unit of type %d in directory", typ)
}

// ParseFile parses a Fukuda .ECG file: header parameters, acquisition datetime,
// and the decoded waveforms.
func ParseFile(dat []byte) (*FukudaData, error) {
	u, err := findUnit(dat, unitWave)
	if err != nil {
		return nil, err
	}
	if u+waveCodes > len(dat) {
		return nil, fmt.Errorf("waveform unit truncated")
	}
	be16 := func(off int) int { return int(binary.BigEndian.Uint16(dat[u+off:])) }
	interval := be16(waveInterval)
	avm := be16(waveScale)
	nSamples := int(binary.BigEndian.Uint32(dat[u+waveSamples:]))
	compressed := be16(waveCompress) != 0
	nLeads := be16(waveLeads)
	if interval <= 0 || nSamples <= 0 || nLeads <= 0 {
		return nil, fmt.Errorf("implausible header: interval=%d nSamples=%d nLeads=%d", interval, nSamples, nLeads)
	}
	if nLeads > maxWaveLeads {
		return nil, fmt.Errorf("unsupported waveform layout: %d leads (max %d)", nLeads, maxWaveLeads)
	}
	dataOff := u + waveCodes + 2*nLeads + 4
	if dataOff > len(dat) {
		return nil, fmt.Errorf("waveform unit truncated")
	}
	names := make([]string, nLeads)
	for i := range names {
		code := uint16(be16(waveCodes + 2*i))
		name, ok := leadCodeName[code]
		if !ok {
			return nil, fmt.Errorf("unknown lead code %d", code)
		}
		names[i] = name
	}
	length := int(binary.BigEndian.Uint32(dat[dataOff-4:]))
	if dataOff+length > len(dat) {
		return nil, fmt.Errorf("%w: waveform data length %d runs past end of file", ErrTruncatedStream, length)
	}
	stream := dat[dataOff : dataOff+length]

	fd := &FukudaData{
		Record: RecordParams{
			SampleRate:   1_000_000 / interval,
			TotalSamples: nSamples,
			NumLeads:     nLeads,
			Scale:        float64(avm) / 1000.0,
		},
		Patient: PatientData{
			RecordingAt: findDatetime(dat),
			DeviceModel: findDeviceModel(dat),
		},
		Measurement: findMeasurements(dat),
	}
	fillDemographics(dat, &fd.Patient)

	var decoded [][]int32
	if compressed {
		decoded, err = DecodeAll(stream, 0, nLeads, nSamples)
	} else {
		decoded, err = rawLeads(stream, nLeads, nSamples)
	}
	if err != nil {
		return nil, err
	}
	fd.Leads = make(map[string][]int32, nLeads)
	for i, name := range names {
		fd.Leads[name] = decoded[i]
	}
	if fd.Leads["I"] == nil || fd.Leads["II"] == nil {
		return nil, fmt.Errorf("leads I and II are required to derive the limb leads")
	}
	return fd, nil
}

// rawLeads reads an uncompressed waveform (compression flag 0): the DLL copies
// it as-is, as 16-bit samples in the same big-endian words as the Huffman
// stream, with the same lead split.
// ponytail: no uncompressed sample file to confirm byte order; BE inferred from
// Nth_UntSwapCtl swapping the unit before both paths.
func rawLeads(stream []byte, nLeads, nSamples int) ([][]int32, error) {
	flat := make([]int32, len(stream)/2)
	for i := range flat {
		flat[i] = int32(int16(binary.BigEndian.Uint16(stream[2*i:])))
	}
	if len(flat) < nLeads*nSamples {
		return nil, fmt.Errorf("%w: raw waveform holds %d of %d samples", ErrTruncatedStream, len(flat), nLeads*nSamples)
	}
	return splitLeads(flat, nLeads, nSamples), nil
}

// findDatetime scans for the acquisition timestamp, stored as six big-endian
// uint16 fields (year, month, day, hour, minute, second). Returns the zero time
// if no plausible timestamp is present.
func findDatetime(dat []byte) time.Time {
	for i := 0; i+12 <= len(dat); i += 2 {
		y := int(binary.BigEndian.Uint16(dat[i : i+2]))
		if y < 1990 || y > 2100 {
			continue
		}
		mo := int(binary.BigEndian.Uint16(dat[i+2 : i+4]))
		d := int(binary.BigEndian.Uint16(dat[i+4 : i+6]))
		h := int(binary.BigEndian.Uint16(dat[i+6 : i+8]))
		mi := int(binary.BigEndian.Uint16(dat[i+8 : i+10]))
		s := int(binary.BigEndian.Uint16(dat[i+10 : i+12]))
		if mo >= 1 && mo <= 12 && d >= 1 && d <= 31 && h < 24 && mi < 60 && s < 60 {
			return time.Date(y, time.Month(mo), d, h, mi, s, 0, time.UTC)
		}
	}
	return time.Time{}
}

// measFF is the 0xFF run that pads the header ahead of the measurement block.
var measFF = []byte{0xFF, 0xFF, 0xFF, 0xFF}

// findMeasurements locates and parses the analytical measurement block. The
// block follows a run of 0xFF padding in the header; fields are big-endian
// uint16 at fixed offsets from the block start:
//
//	+0 heart rate (bpm)   +4 PR (ms)    +6 QRS (ms)
//	+8 QT (ms)            +10 QTc (ms)  +14 QRS axis (deg)
//
// Field offsets and the layout were confirmed against the reference measurement
// values of a paired recording. Returns a zero MeasurementData if not found.
func findMeasurements(dat []byte) MeasurementData {
	for i := 0; i+len(measFF) <= len(dat); i++ {
		if !bytes.Equal(dat[i:i+len(measFF)], measFF) {
			continue
		}
		b := i + len(measFF)
		if b+16 > len(dat) {
			continue
		}
		u := func(off int) int { return int(binary.BigEndian.Uint16(dat[b+off : b+off+2])) }
		hr, pr, qrs, qt, qtc := u(0), u(4), u(6), u(8), u(10)
		// Plausibility gate: all core intervals must be physiologic together.
		if hr < 20 || hr > 300 || pr < 40 || pr > 400 || qrs < 40 || qrs > 250 ||
			qt < 150 || qt > 700 || qtc < 150 || qtc > 700 {
			continue
		}
		axis := u(14)
		return MeasurementData{
			HeartRate:   hr,
			PRInterval:  pr,
			QRSDuration: qrs,
			QTInterval:  qt,
			QTcInterval: qtc,
			QRSAxis:     int(int16(axis)),
			HasQRSAxis:  true,
		}
	}
	return MeasurementData{}
}

// Demographic field offsets in the FX-series header. These are fixed absolute
// offsets recovered by reverse-engineering [RE] — they hold across the reference
// set but may shift on a different firmware; every read is range/printable
// checked so a mismatch yields an empty field rather than garbage.
const (
	offPatientID = 688  // null-terminated ASCII, "<id>) …"
	offSex       = 1604 // BE uint16: 1=Male; 2=Female assumed (unverified — all reference files are Male)
	offBirthYear = 1610 // BE uint16
	offBirthMon  = 1612 // BE uint16
	offBirthDay  = 1614 // BE uint16
	offName      = 1650 // null-terminated ASCII
)

// fillDemographics reads patient name, ID, sex and birth date from their fixed
// header offsets, leaving a field empty when the stored value is missing or
// implausible.
func fillDemographics(dat []byte, p *PatientData) {
	p.FamilyName = readCString(dat, offName)
	if id := readCString(dat, offPatientID); id != "" {
		// "<id>) <composite>" — keep the leading token as the patient ID.
		if i := strings.IndexAny(id, ") \t"); i > 0 {
			id = id[:i]
		}
		p.PatientID = id
	}
	if offSex+2 <= len(dat) {
		switch binary.BigEndian.Uint16(dat[offSex : offSex+2]) {
		case 1:
			p.Gender = "M"
		case 2:
			p.Gender = "F"
		}
	}
	if offBirthDay+2 <= len(dat) {
		y := int(binary.BigEndian.Uint16(dat[offBirthYear : offBirthYear+2]))
		mo := int(binary.BigEndian.Uint16(dat[offBirthMon : offBirthMon+2]))
		d := int(binary.BigEndian.Uint16(dat[offBirthDay : offBirthDay+2]))
		if y >= 1900 && y <= 2100 && mo >= 1 && mo <= 12 && d >= 1 && d <= 31 {
			p.BirthDate = fmt.Sprintf("%04d%02d%02d", y, mo, d)
		}
	}
}

// readCString returns the printable, null-terminated ASCII string at off, or ""
// if it is absent, empty, or contains non-printable bytes.
func readCString(dat []byte, off int) string {
	if off < 0 || off >= len(dat) {
		return ""
	}
	end := off
	for end < len(dat) && dat[end] != 0 {
		if dat[end] < 0x20 || dat[end] > 0x7e {
			return ""
		}
		end++
	}
	return strings.TrimSpace(string(dat[off:end]))
}

// findDeviceModel returns the Fukuda device model string if present (e.g.
// "FX-8322"), else "".
func findDeviceModel(dat []byte) string {
	if i := bytes.Index(dat, []byte("FX-")); i >= 0 {
		end := i
		for end < len(dat) && end < i+16 && dat[end] > 0x20 && dat[end] < 0x7f {
			end++
		}
		return string(dat[i:end])
	}
	return ""
}

package fukudatofda

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// dataMarker precedes the compressed rhythm block and its parameter header.
var dataMarker = []byte{0x00, 0x09, 0x02}

// leadOrder is the on-file order of the 8 measured leads in the bitstream.
var leadOrder = []string{"I", "II", "V1", "V2", "V3", "V4", "V5", "V6"}

// ParseFile parses a Fukuda .ECG file: header parameters, acquisition datetime,
// and the decoded waveforms.
func ParseFile(dat []byte) (*FukudaData, error) {
	mk := bytes.LastIndex(dat, dataMarker)
	if mk < 0 || mk+44 > len(dat) {
		return nil, fmt.Errorf("data marker 00 09 02 not found")
	}
	interval := int(binary.BigEndian.Uint16(dat[mk+10 : mk+12])) // µs/sample
	avm := int(binary.BigEndian.Uint16(dat[mk+12 : mk+14]))      // nV/digit
	nSamples := int(binary.BigEndian.Uint32(dat[mk+14 : mk+18]))
	nLeads := int(binary.BigEndian.Uint16(dat[mk+20 : mk+22]))
	if interval <= 0 || nSamples <= 0 || nLeads <= 0 || nLeads > len(leadOrder) {
		return nil, fmt.Errorf("implausible header: interval=%d nSamples=%d nLeads=%d", interval, nSamples, nLeads)
	}

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

	startBit := (mk + 44) * 8
	decoded, err := DecodeAll(dat, startBit, nLeads, nSamples)
	if err != nil {
		return nil, err
	}
	fd.Leads = make(map[string][]int32, nLeads)
	for i := 0; i < nLeads; i++ {
		fd.Leads[leadOrder[i]] = decoded[i]
	}
	return fd, nil
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

package mfertofda

import (
	"strings"
	"time"

	"github.com/LIRYC-IHU/ecg-bridge/metaject"
)

// MferData is the intermediate representation parsed from an MFER (.mwf) file
// (Medical waveform Format Encoding Rules), ready to be converted to FDA aECG
// XML, DICOM or PDF.
type MferData struct {
	// Device
	Manufacturer string // e.g. "Nihon Kohden"
	ModelName    string // e.g. "2350K"
	SoftwareVer  string // e.g. "03.04" (from the manufacturer frame)

	// Study / acquisition
	StudyDate string // YYYYMMDD
	StudyTime string // HHMMSS
	StudyUID  string // from the UID frame, or generated

	// Patient (often absent from the .mwf — carried by the companion CDA XML)
	PatientID   string
	PatientName string // "LAST^FIRST"
	PatientSex  string // "M" / "F" / ""
	BirthDate   string // YYYYMMDD if known, else ""

	// Signal characteristics
	SampleRate  float64 // Hz, derived from the sampling-interval frame (0x0B)
	NumChannels int     // stored leads (8: I, II, V1-V6)
	Scale       float64 // µV per LSB (1.25 for NK MFER)

	// Filters (from comment frames, 0x11)
	FilterHPF   float64 // high-pass cutoff (Hz)
	NotchFilter float64 // hum/AC filter (Hz)

	// Rhythm waveform (ORIGINAL) — 12 leads, 4 of them derived.
	RhythmLeads [12][]int16
	// Median/representative beat (DERIVED) — 12 leads.
	MedianLeads [12][]int16
}

// Anonymize blanks the direct patient identifiers (name, ID, birth date) while
// keeping clinically useful fields (sex, acquisition dates, signal). NK MFER
// .mwf files usually carry no patient identity at all, so this is often a no-op.
func (d *MferData) Anonymize() {
	d.PatientName = ""
	d.PatientID = ""
	d.BirthDate = ""
}

// ApplyMetadata overwrites patient-identity and study-date fields from ov.
// Only fields present in ov are applied; nil fields leave the parsed value.
// This is the primary way to populate patient demographics for an MFER file,
// since the binary itself rarely carries them.
func (d *MferData) ApplyMetadata(ov *metaject.Override) {
	if ov == nil {
		return
	}
	if ov.PatientID != nil {
		d.PatientID = *ov.PatientID
	}
	if ov.PatientName != nil {
		d.PatientName = *ov.PatientName
	}
	if ov.Gender != nil {
		d.PatientSex = *ov.Gender
	}
	if ov.BirthDate != nil {
		d.BirthDate = *ov.BirthDate
	}
	if ov.Datetime != nil {
		d.StudyDate, d.StudyTime = metaject.SplitDatetime(*ov.Datetime)
	}
}

// splitDeviceField parses the MFER manufacturer frame (tag 0x17), formatted as
// "MANUFACTURER^MODEL^VERSION" (e.g. "NIHON KOHDEN^2350K^03.04").
func splitDeviceField(s string) (manufacturer, model, version string) {
	parts := strings.Split(strings.TrimSpace(s), "^")
	get := func(i int) string {
		if i < len(parts) {
			return strings.TrimSpace(parts[i])
		}
		return ""
	}
	return get(0), get(1), get(2)
}

// normalizeManufacturer maps the raw MFER manufacturer string to a canonical
// vendor name for the FDA/DICOM device author.
func normalizeManufacturer(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), "NIHON KOHDEN") {
		return "Nihon Kohden"
	}
	return strings.TrimSpace(raw)
}

// mferTime decodes the MFER measurement-time frame (tag 0x85): a 7-byte value
// [year u16 LE, month, day, hour, minute, second].
func mferTime(b []byte, littleEndian bool) (date, tm string, ok bool) {
	if len(b) < 7 {
		return "", "", false
	}
	var year int
	if littleEndian {
		year = int(b[0]) | int(b[1])<<8
	} else {
		year = int(b[0])<<8 | int(b[1])
	}
	mo, da, ho, mi, se := int(b[2]), int(b[3]), int(b[4]), int(b[5]), int(b[6])
	if year < 1800 || year > 2200 || mo < 1 || mo > 12 || da < 1 || da > 31 {
		return "", "", false
	}
	t := time.Date(year, time.Month(mo), da, ho, mi, se, 0, time.UTC)
	return t.Format("20060102"), t.Format("150405"), true
}

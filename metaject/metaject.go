// Package metaject defines the JSON metadata-injection format shared by every
// converter binary. When a converter receives JSON on stdin (piped, non-empty),
// the provided fields overwrite the patient-identity / study-date metadata
// parsed from the source file before the output is built.
//
// Semantics: a field present in the JSON (even an empty string) overwrites the
// parsed value; a field absent from the JSON leaves the parsed value untouched.
// This is why every field is a pointer — nil means "absent", non-nil means "set".
//
// Example:
//
//	echo '{"patientID":"12345","patientName":"DOE^John","gender":"M"}' \
//	  | muse-to-fda -i ecg.xml -o ecg_fda.xml
package metaject

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Override is the canonical injectable metadata format. Every field is optional
// (pointer): nil = absent (keep parsed value), non-nil = overwrite (even with "").
//
// Modules use the subset of fields relevant to their format:
//   - Single-name formats (muse, philips, mindray, fda) use PatientName ("LAST^FIRST").
//   - Nihon Kohden uses FamilyName / GivenName (or splits PatientName on "^").
//   - Datetime is the acquisition/study instant, "YYYYMMDDHHMMSS".
type Override struct {
	PatientID   *string `json:"patientID,omitempty"`
	PatientName *string `json:"patientName,omitempty"` // "LAST^FIRST"
	FamilyName  *string `json:"familyName,omitempty"`  // NK split name
	GivenName   *string `json:"givenName,omitempty"`   // NK split name
	Gender      *string `json:"gender,omitempty"`      // "M" / "F" / ...
	Age         *string `json:"age,omitempty"`         // e.g. "055Y"
	BirthDate   *string `json:"birthDate,omitempty"`   // "YYYYMMDD"
	Datetime    *string `json:"datetime,omitempty"`    // "YYYYMMDDHHMMSS"
}

// Parse decodes JSON bytes into an Override. Unknown keys are rejected so a
// typo in a field name fails loudly instead of being silently ignored.
func Parse(data []byte) (*Override, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var ov Override
	if err := dec.Decode(&ov); err != nil {
		return nil, fmt.Errorf("invalid injection JSON: %w", err)
	}
	return &ov, nil
}

// FromStdin returns the Override piped on stdin, or nil when stdin is a terminal
// or carries no data. This lets `echo '{...}' | converter` inject metadata while
// a plain `converter` invocation is unaffected.
func FromStdin() (*Override, error) {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return nil, nil //nolint:nilerr // no stdin → no injection
	}
	// A character device is an interactive terminal — nothing was piped.
	if fi.Mode()&os.ModeCharDevice != 0 {
		return nil, nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	return Parse(data)
}

// SplitDatetime splits "YYYYMMDDHHMMSS" into a date (YYYYMMDD) and time (HHMMSS)
// part, the layout used by the FDA/DICOM string fields. A short value yields a
// best-effort split (date = first 8 chars, time = remainder).
func SplitDatetime(s string) (date, t string) {
	if len(s) >= 8 {
		return s[:8], s[8:]
	}
	return s, ""
}

// ParseDatetime parses "YYYYMMDDHHMMSS" (or the date-only "YYYYMMDD") into a
// time.Time. The second return value is false when s is empty or unparseable.
func ParseDatetime(s string) (time.Time, bool) {
	for _, layout := range []string{"20060102150405", "20060102"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

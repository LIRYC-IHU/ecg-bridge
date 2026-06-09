package dicomconf

// scpecgLeadCodes maps the standard 12-lead names to their SCP-ECG lead
// identification CodeValue (DICOM "SCPECG" coding scheme).
//
// Per the SCP-ECG lead table the numbering is NOT sequential 1..12: the limb
// and precordial leads come first (I=1, II=2, V1..V6=3..8) and the derived
// leads are 61..64 (III=61, aVR=62, aVL=63, aVF=64). Keeping this in one place
// avoids the per-converter copies drifting out of sync.
var scpecgLeadCodes = map[string]string{
	"I":   "5.6.3-9-1",
	"II":  "5.6.3-9-2",
	"V1":  "5.6.3-9-3",
	"V2":  "5.6.3-9-4",
	"V3":  "5.6.3-9-5",
	"V4":  "5.6.3-9-6",
	"V5":  "5.6.3-9-7",
	"V6":  "5.6.3-9-8",
	"III": "5.6.3-9-61",
	"aVR": "5.6.3-9-62",
	"aVL": "5.6.3-9-63",
	"aVF": "5.6.3-9-64",
}

// SCPECGLeadCode returns the SCP-ECG CodeValue for a 12-lead name (e.g. "aVR"),
// or "" if the name is unknown.
func SCPECGLeadCode(lead string) string {
	return scpecgLeadCodes[lead]
}

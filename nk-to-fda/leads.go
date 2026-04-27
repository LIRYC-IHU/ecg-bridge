package nktofda

import "github.com/LIRYC-IHU/hl7v3-aecg/hl7aecg/types"

// DeriveLeads computes the 4 augmented leads from I and II.
// III = II - I
// aVR = -(I + II) / 2
// aVL = (2I - II) / 2
// aVF = (2II - I) / 2
func DeriveLeads(i, ii []int32) (iii, avr, avl, avf []int32) {
	n := len(i)
	iii = make([]int32, n)
	avr = make([]int32, n)
	avl = make([]int32, n)
	avf = make([]int32, n)
	for t := 0; t < n; t++ {
		iii[t] = ii[t] - i[t]
		avr[t] = -(i[t] + ii[t]) / 2
		avl[t] = (2*i[t] - ii[t]) / 2
		avf[t] = (2*ii[t] - i[t]) / 2
	}
	return
}

// leadNameToCode maps NK lead names to hl7aecg LeadCode.
var leadNameToCode = map[string]types.LeadCode{
	"I":   types.MDC_ECG_LEAD_I,
	"II":  types.MDC_ECG_LEAD_II,
	"III": types.MDC_ECG_LEAD_III,
	"aVR": types.MDC_ECG_LEAD_AVR,
	"aVL": types.MDC_ECG_LEAD_AVL,
	"aVF": types.MDC_ECG_LEAD_AVF,
	"V1":  types.MDC_ECG_LEAD_V1,
	"V2":  types.MDC_ECG_LEAD_V2,
	"V3":  types.MDC_ECG_LEAD_V3,
	"V4":  types.MDC_ECG_LEAD_V4,
	"V5":  types.MDC_ECG_LEAD_V5,
	"V6":  types.MDC_ECG_LEAD_V6,
}

// Build12LeadMap builds the map[LeadCode][]int required by hl7aecg from 8 measured leads.
func Build12LeadMap(leads map[string][]int32) map[types.LeadCode][]int {
	i := leads["I"]
	ii := leads["II"]
	iii, avr, avl, avf := DeriveLeads(i, ii)

	all := map[string][]int32{
		"I":   i,
		"II":  ii,
		"III": iii,
		"aVR": avr,
		"aVL": avl,
		"aVF": avf,
		"V1":  leads["V1"],
		"V2":  leads["V2"],
		"V3":  leads["V3"],
		"V4":  leads["V4"],
		"V5":  leads["V5"],
		"V6":  leads["V6"],
	}

	result := make(map[types.LeadCode][]int, 12)
	for name, samples := range all {
		lc, ok := leadNameToCode[name]
		if !ok {
			continue
		}
		intSamples := make([]int, len(samples))
		for j, s := range samples {
			intSamples[j] = int(s)
		}
		result[lc] = intSamples
	}
	return result
}

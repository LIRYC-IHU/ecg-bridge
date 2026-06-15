package ecgpdf

import "strings"

// labels holds every static UI string of the report in one language. Units
// (bpm, ms, mV, °, mm/mV…) are language-neutral and stay inline in the renderer.
type labels struct {
	id, name, sex, dob                string
	ageSuffix, cmSuffix, kgSuffix, bp string
	meds, symptoms, history           string
	hr, prInt, qrsDur                 string
	qtQtc, axis, amplDiv, amplSum     string
	filter                            string
	service, exam                     string
	diag, physician, dateL            string
}

var labelsFR = labels{
	id: "ID:", name: "Nom:", sex: "Sexe:", dob: "Date naiss:",
	ageSuffix: " ans", cmSuffix: " cm", kgSuffix: " kg", bp: "____ mmHg",
	meds: "Medicament:", symptoms: "Symptomes:", history: "Historiqu:",
	hr: "Frequence ventriculaire:", prInt: "intervalle PR:", qrsDur: "duree QRS:",
	qtQtc: "int. QT/QTc:", axis: "axe P/QRS/T:",
	amplDiv: "ampl. RV5/SV1:", amplSum: "ampl. RV5+SV1:",
	filter:  "Filtre:",
	service: "Service:", exam: "Exam:",
	diag: "Diagnostic médecin:", physician: "Médecin:", dateL: "Date:",
}

var labelsEN = labels{
	id: "ID:", name: "Name:", sex: "Sex:", dob: "DOB:",
	ageSuffix: " yrs", cmSuffix: " cm", kgSuffix: " kg", bp: "____ mmHg",
	meds: "Medication:", symptoms: "Symptoms:", history: "History:",
	hr: "Ventricular rate:", prInt: "PR interval:", qrsDur: "QRS duration:",
	qtQtc: "QT/QTc:", axis: "P/QRS/T axis:",
	amplDiv: "ampl. RV5/SV1:", amplSum: "ampl. RV5+SV1:",
	filter:  "Filter:",
	service: "Dept:", exam: "Exam:",
	diag: "Physician diagnosis:", physician: "Physician:", dateL: "Date:",
}

func labelsFor(lang string) labels {
	if normalizeLang(lang) == "fr" {
		return labelsFR
	}
	return labelsEN
}

// normalizeLang returns "fr" or "en" (default), matching the converter's own
// language fallback so the report and the statement vocabulary stay in sync.
func normalizeLang(lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	if len(l) > 2 {
		l = l[:2]
	}
	if l == "fr" {
		return "fr"
	}
	return "en"
}

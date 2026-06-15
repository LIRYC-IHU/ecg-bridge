package main

import nktofda "converter-fda/nk-to-fda"

// labels holds every static UI string of the report in one language. Units
// (bpm, ms, mV, mm/mV…) are language-neutral and stay inline in the renderer.
type labels struct {
	id, name, sex, dob          string
	ageSuffix, cmSuffix, kgSuffix, bp string
	meds, symptoms, history     string
	hr, prInt, qrsDur           string
	qtQtc, axis, amplDiv, amplSum string
	scaleNote                   string
	service, exam               string
}

var labelsFR = labels{
	id: "ID:", name: "Nom:", sex: "Sexe:", dob: "Date naiss:",
	ageSuffix: " ans", cmSuffix: " cm", kgSuffix: " kg", bp: "____ mmHg",
	meds: "Medicament:", symptoms: "Symptomes:", history: "Historiqu:",
	hr: "Frequence ventriculaire:", prInt: "intervalle PR:", qrsDur: "duree QRS:",
	qtQtc: "int. QT/QTc:", axis: "axe P/QRS/T:",
	amplDiv: "ampl. RV5/SV1:", amplSum: "ampl. RV5+SV1:",
	scaleNote: "10 mm/mV   25 mm/s   Filtre: H50 a 150 Hz",
	service:   "Service:", exam: "Exam:",
}

var labelsEN = labels{
	id: "ID:", name: "Name:", sex: "Sex:", dob: "DOB:",
	ageSuffix: " yrs", cmSuffix: " cm", kgSuffix: " kg", bp: "____ mmHg",
	meds: "Medication:", symptoms: "Symptoms:", history: "History:",
	hr: "Ventricular rate:", prInt: "PR interval:", qrsDur: "QRS duration:",
	qtQtc: "QT/QTc:", axis: "P/QRS/T axis:",
	amplDiv: "ampl. RV5/SV1:", amplSum: "ampl. RV5+SV1:",
	scaleNote: "10 mm/mV   25 mm/s   Filter: H50 to 150 Hz",
	service:   "Dept:", exam: "Exam:",
}

// labelsFor returns the label set for lang, reusing the converter's
// NormalizeLang so the proto and the statement vocabulary stay in sync
// (anything other than "fr"/"en" falls back to English, like StatementText).
func labelsFor(lang string) labels {
	if nktofda.NormalizeLang(lang) == "fr" {
		return labelsFR
	}
	return labelsEN
}

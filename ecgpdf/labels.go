package ecgpdf

import "strings"

// labels holds every static UI string of the report in one language. Units
// (bpm, ms, mV, °, mm/mV…) are language-neutral and stay inline in the renderer.
type labels struct {
	id, name, sex, dob                string
	ageSuffix, cmSuffix, kgSuffix, bp string
	meds, symptoms, history           string
	recordedAt                        string
	hr, prInt, qrsDur                 string
	qtQtc, axis, amplDiv              string
	filter, notInSource               string
	interpDevice, interpStatus        string
	service, exam                     string
	diag, physician, dateL            string
	// disclaimer states what the document is and what it is not. It must stay
	// consistent with the project's declared intended use — the same statement
	// carried by the README and by the ANSM filing. The previous wording said
	// the document was "not for diagnostic use", which contradicted a filing
	// describing this PDF as intended for reading by a health professional.
	// A contradiction between the two is worse than either alone.
	disclaimer string
}

var labelsFR = labels{
	id: "ID:", name: "Nom:", sex: "Sexe:", dob: "Date naiss:",
	ageSuffix: " ans", cmSuffix: " cm", kgSuffix: " kg", bp: "____ mmHg",
	meds: "Medicament:", symptoms: "Symptomes:", history: "Historique:",
	recordedAt: "Enregistré le:",
	hr:         "Frequence ventriculaire:", prInt: "intervalle PR:", qrsDur: "duree QRS:",
	qtQtc: "int. QT/QTc:", axis: "axe P/QRS/T:",
	amplDiv: "ampl. RV5/SV1:",
	filter:  "Filtre:", notInSource: "non renseigné par le fichier source",
	interpDevice: "Interprétation produite par l'appareil:",
	interpStatus: "Statut d'interprétation:",
	service:      "Service:", exam: "Exam:",
	diag: "Diagnostic médecin:", physician: "Médecin:", dateL: "Date:",
	disclaimer: "Document produit par conversion automatique du fichier de l'appareil d'acquisition. " +
		"Le convertisseur ne réalise aucune mesure ni interprétation : les valeurs et énoncés reportés sont ceux de l'appareil. " +
		"Ne se substitue pas à l'appareil d'acquisition pour l'établissement du diagnostic.",
}

var labelsEN = labels{
	id: "ID:", name: "Name:", sex: "Sex:", dob: "DOB:",
	ageSuffix: " yrs", cmSuffix: " cm", kgSuffix: " kg", bp: "____ mmHg",
	meds: "Medication:", symptoms: "Symptoms:", history: "History:",
	recordedAt: "Recorded:",
	hr:         "Ventricular rate:", prInt: "PR interval:", qrsDur: "QRS duration:",
	qtQtc: "QT/QTc:", axis: "P/QRS/T axis:",
	amplDiv: "ampl. RV5/SV1:",
	filter:  "Filter:", notInSource: "not stated in the source file",
	interpDevice: "Interpretation produced by device:",
	interpStatus: "Interpretation status:",
	service:      "Dept:", exam: "Exam:",
	diag: "Physician diagnosis:", physician: "Physician:", dateL: "Date:",
	disclaimer: "Document produced by automatic conversion of the acquisition device's file. " +
		"The converter performs no measurement and no interpretation: the values and statements shown are the acquiring device's. " +
		"Does not replace the acquisition device for establishing a diagnosis.",
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

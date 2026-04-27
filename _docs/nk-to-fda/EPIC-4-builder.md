# EPIC-4 — FDA aECG XML Builder

## Objectif

Construire le fichier XML HL7 aECG (FDA 21 CFR Part 11) à partir des données NK parsées,
en utilisant la bibliothèque `github.com/LIRYC-IHU/hl7v3-aecg`.

## Référence XML

Voir `data_nk/250606392_20250910135412.FDA.xml` — structure attendue :

```xml
<AnnotatedECG>
  <id root="..." />
  <code code="93000" ... />
  <effectiveTime><low value="20250910135412" /></effectiveTime>
  <componentOf>
    <timepointEvent>
      <componentOf>
        <subjectAssignment>
          <subject><trialSubject>
            <id ... extension="250606392" />
            <subjectDemographicPerson>
              <name>feugueur feugueur</name>
              <administrativeGenderCode code="F" ... />
              <birthTime value="19630526" />
            </subjectDemographicPerson>
          </trialSubject></subject>
          <componentOf><clinicalTrial>
            <location><trialSite><location><name>1453 - 3E EST</name></location>
            </trialSite></location>
          </clinicalTrial></componentOf>
        </subjectAssignment>
      </componentOf>
    </timepointEvent>
  </componentOf>
  <controlVariable> ... HR, PR, QRS, QT, QTc, axes, amplitudes </controlVariable>
  <component>
    <series classCode="OBSSER"> <!-- RHYTHM, 12 leads × 5000 samples -->
```

## User Stories

### US-4.1 — Initialiser le document aECG

**Critères d'acceptation :**
- [ ] `h := hl7aecg.NewHl7xml("")`
- [ ] `h.Initialize(types.CPT_CODE_ECG_Routine, types.CPT_OID, "CPT-4", "")`
- [ ] `h.HL7AEcg.ConfidentialityCode = nil`
- [ ] `h.HL7AEcg.ReasonCode = nil`
- [ ] Root ID = UUID généré (pas de StudyUID NK disponible)

### US-4.2 — Section sujet/patient

**Critères d'acceptation :**
- [ ] `h.SetSubject(rootUUID, patientID, types.SUBJECT_ROLE_ENROLLED)`
- [ ] `h.SetSubjectDemographics(fullName, patientID, gender, birthDate, race)`
- [ ] Name format : "familyName givenName" (espace entre les deux)
- [ ] Si birthDate vide : `sdp.BirthTime = nil`
- [ ] Location → `h.SetLocation(...)` avec siteName = location field

### US-4.3 — ClinicalTrial et effectiveTime

**Critères d'acceptation :**
- [ ] `ct.SetID(rootUUID, filename+"-"+datetime)` où filename = base du .DAT
- [ ] effectiveTime low = datetime NK (format `YYYYMMDDHHmmss`)
- [ ] effectiveTime high = datetime + 10s (5000 samples @ 500 Hz = 10s)

### US-4.4 — ControlVariables (mesures)

**Critères d'acceptation :**
- [ ] HeartRate (LOINC 11328-2, bpm) si > 0
- [ ] PRInterval (LOINC 8625-6, ms) si > 0
- [ ] QRSDuration (LOINC 8633-0, ms) si > 0
- [ ] QTInterval (LOINC 8634-8, ms) si > 0
- [ ] QTcInterval (LOINC 8636-3, ms) si > 0
- [ ] PAxis (LOINC 8626-4, deg)
- [ ] QRSAxis (LOINC 8632-2, deg)
- [ ] TAxis (LOINC 8638-9, deg)
- [ ] V5RAmplitude (LOINC 9995-2, µV) si > 0
- [ ] V1SAmplitude (LOINC 10040-4, µV) si > 0
- [ ] Age (LOINC 21612-7, a) si disponible

**Note :** les axes signés (négatifs) passent via `AddAnnotation` MDC, les positifs aussi.
Utiliser `annSet.AddAnnotation(code, oid, value, unit)` directement pour les axes.

### US-4.5 — Série rythme (RHYTHM, 12 leads)

**Critères d'acceptation :**
- [ ] `h.AddRhythmSeries(startDT, endDT, nil, nil, 500, leads12, 0.0, 1.25)`
  - baseline = 0.0 µV
  - scale = 1.25 µV/digit (confirmé via FDA XML: `scale=1.250000`)
- [ ] leads12 : map `types.LeadCode → []int` pour les 12 leads
- [ ] Valeurs int = samples int32 castés en int

### US-4.6 — Device author

**Critères d'acceptation :**
- [ ] `lastComp.Series.Author` renseigné avec modèle NK (ex: "2350K")
- [ ] Manufacturer = "Nihon Kohden" (valeur fixe)

### US-4.7 — Sérialisation et validation

**Critères d'acceptation :**
- [ ] Valider avec `h.HL7AEcg.Validate(ctx, vctx)` avant sérialisation
- [ ] `stdxml.MarshalIndent(h.HL7AEcg, "", "  ")`
- [ ] Préfixe `stdxml.Header` (`<?xml version="1.0" encoding="UTF-8"?>`)

## Méthodes hl7v3-aecg manquantes potentielles

À vérifier lors de l'implémentation et reporter dans cette section.

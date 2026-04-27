# EPIC-6 — Tests & Validation

## Objectif

Garantir la correction du décodeur et du builder via des tests unitaires et un test
d'intégration end-to-end contre le fichier de référence.

## User Stories

### US-6.1 — Tests unitaires du parser

**Critères d'acceptation :**
- [ ] Test `ParseSections` : vérifier offsets détectés dans `00000005.DAT`
- [ ] Test `ParsePatient` : vérifier FamilyName="feugueur", PatientID="250606392", datetime
- [ ] Test `ParseMeasurement` : vérifier HR=105, PR=260, QRS=94, QT=374, QTc=435, PAxis=28

### US-6.2 — Tests unitaires du décodeur

**Critères d'acceptation :**
- [ ] Test mode 0 : 5 samples simples → output exact
- [ ] Test mode 1 (count < 5) : simple copy
- [ ] Test mode 1 (count ≥ 5) : vérifier interpolation midpoint sur exemple Python
- [ ] Test `parse_code_table` : vérifier taille et premier symbole sur un frame connu

### US-6.3 — Test d'intégration end-to-end (waveform)

**Critères d'acceptation :**
- [ ] `t.Skip` si fichier `data_nk/00000005.DAT` absent
- [ ] Décoder 8 leads → comparer sample-par-sample avec FDA XML
- [ ] Résultat attendu : 0 différence pour I, II, V1–V6 (5000 samples chacun)

### US-6.4 — Test d'intégration FDA XML

**Critères d'acceptation :**
- [ ] `t.Skip` si fichiers de référence absents
- [ ] Convert("data_nk/00000005.DAT", "/tmp/test_nk.xml") → pas d'erreur
- [ ] XML produit contient les balises essentielles : `<AnnotatedECG>`, `<series>`, `<digits>`
- [ ] HR=105, PR=260, QRS=94, QT=374 présents dans les `<controlVariable>`

## Fichiers de test requis

| Fichier | Skip si absent |
|---------|---------------|
| `data_nk/00000005.DAT` | Oui |
| `data_nk/250606392_20250910135412.FDA.xml` | Oui (intégration waveform uniquement) |

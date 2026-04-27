# NK to FDA — Product Requirements Document

## Objectif

Convertir les fichiers ECG propriétaires Nihon Kohden (`.DAT` — format PEC binaire) en fichiers
HL7 aECG XML (FDA 21 CFR Part 11) en utilisant la bibliothèque locale `hl7v3-aecg`.

## Contexte technique

- Format source : **PEC** (Cardiology PEC File) — conteneur binaire sectionné avec checksums
- Compression waveform : **Type 16** — Huffman VLC + cumsum global + mode handlers
- Décompresseur Python de référence : `data_nk/nk_decoder_v6.py` (100% exact vs FDA XML)
- 8 leads mesurés (I, II, V1–V6), 4 dérivés (III, aVR, aVL, aVF)
- Format cible : HL7 aECG XML (`/AnnotatedECG/`), bibliothèque Go : `github.com/LIRYC-IHU/hl7v3-aecg`

## Fichiers de référence

| Fichier | Rôle |
|---------|------|
| `data_nk/00000005.DAT` | Fichier NK de test |
| `data_nk/250606392_20250910135412.FDA.xml` | Sortie FDA attendue (ground truth) |
| `data_nk/NK-Binary-Format-RE.md` | Documentation reverse-engineering du format |
| `data_nk/nk_decoder_v6.py` | Décodeur waveform Python de référence |

## Epics

| Epic | Titre | Statut |
|------|-------|--------|
| [EPIC-1](EPIC-1-parser.md) | PEC Binary Parser | À faire |
| [EPIC-2](EPIC-2-decoder.md) | Waveform Decoder (Type 16) | À faire |
| [EPIC-3](EPIC-3-leads.md) | 12-Lead Derivation | À faire |
| [EPIC-4](EPIC-4-builder.md) | FDA aECG XML Builder | À faire |
| [EPIC-5](EPIC-5-cli.md) | CLI Integration | À faire |
| [EPIC-6](EPIC-6-tests.md) | Tests & Validation | À faire |

## Architecture cible

```
cmd/nk-to-fda/main.go          ← CLI cobra
nk-to-fda/
  types.go                     ← NKData, PatientData, MeasurementData
  parser.go                    ← PEC sections → NKData
  decoder.go                   ← Type 16 Huffman → []int32 par lead
  leads.go                     ← III/aVR/aVL/aVF derivation
  converter.go                 ← NKData → FDA XML (hl7v3-aecg)
```

## Champs couverts

### Patient (PATIENT section +0x0000)
| Champ | Offset | Type | Statut |
|-------|--------|------|--------|
| FamilyName | +0x00 | 32B ASCII | Confirmé |
| GivenName | +0x20 | 30B ASCII | Confirmé |
| PatientID | +0x3E | ~10B ASCII | Confirmé |
| Location | +0x143 | null-term ASCII | Confirmé |
| Datetime | +0x16C | year u16 BE + 5B | Confirmé |
| Gender | PATIENT2 +0x7C | u8 (0=?, 1=M, 2=F, 3=U) | Confirmé |
| DOB | inconnu | — | Non trouvé dans binaire |

### Mesures (MEASUREMENT section +0x0008)
| Champ | Offset | Type | Statut |
|-------|--------|------|--------|
| HeartRate | +0x08 | u16 BE | Confirmé |
| PRInterval | +0x0A | u16 BE | Confirmé |
| QRSDuration | +0x0C | u16 BE | Confirmé |
| QTInterval | +0x0E | u16 BE | Confirmé |
| QTcInterval | +0x10 | u16 BE | Confirmé |
| PAxis | +0x12 | i16 BE | Confirmé |
| QRSAxis | +0x14 | i16 BE | Confirmé |
| TAxis | +0x16 | i16 BE | Confirmé |
| V5RAmplitude | +0x18 | u16 BE (nV) | Confirmé |
| V1SAmplitude | +0x1A | u16 BE (nV) | Confirmé |

### Waveform (RECORD section)
| Champ | Offset | Valeur | Statut |
|-------|--------|--------|--------|
| SampleRate | +0x00 | u16 BE | Confirmé (500 Hz) |
| CompressionType | +0x04 | u16 BE = 16 | Confirmé |
| TotalSamples | +0x1C4 | u16 BE | Confirmé (5000) |
| Scale | — | 1.25 µV/digit | Confirmé (FDA XML) |

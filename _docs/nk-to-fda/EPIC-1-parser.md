# EPIC-1 — PEC Binary Parser

## Objectif

Parser le format conteneur PEC du fichier `.DAT` pour extraire toutes les métadonnées
patient, mesures et paramètres d'enregistrement nécessaires à la génération FDA XML.

## User Stories

### US-1.1 — Parser la table des sections

**En tant que** converter,  
**je veux** lire la POINTER section (type 0x0000) pour construire une table d'offsets,  
**afin de** localiser chaque section par type sans scan séquentiel.

**Format section PEC :**
```
+0x00  10B  Preamble (section-specific)
+0x0A  2B   SIZE (u16 BE, inclut le header 14B)
+0x0C  2B   TYPE (u16 BE)
+0x0E  ...  Data (SIZE - 14 bytes)
+SIZE  2B   CRC16
```

**Critères d'acceptation :**
- [ ] Itérer les sections séquentiellement (offset 0 → fin)
- [ ] Construire une map `sectionType → fileOffset`
- [ ] Gérer les types connus : 0x0000, 0x0001, 0x0002, 0x0007, 0x0008, 0x0100, 0x0103, 0x0108, 0x0110, 0x0113, 0x0115, 0x0200
- [ ] Validation: section RECORD (0x0008) obligatoire

### US-1.2 — Parser la section PATIENT (0x0002)

**En tant que** converter,  
**je veux** extraire les champs démographiques depuis la section PATIENT,  
**afin de** renseigner le sujet dans le XML FDA.

**Layout confirmé (data start = section_offset + 14) :**
```
+0x00  32B  FamilyName (null-padded ASCII)
+0x20  30B  GivenName  (null-padded ASCII)
+0x3E  ~10B PatientID  (space-padded ASCII)
+0x143 ~30B Location   (null-terminated ASCII)
+0x16C 7B   Datetime   (year u16 BE, month, day, hour, min, sec)
```

**Critères d'acceptation :**
- [ ] Extraire FamilyName + GivenName, trimmer les nulls/spaces
- [ ] Extraire PatientID (s'arrêter au premier espace/null)
- [ ] Extraire Location
- [ ] Parser Datetime → `time.Time`

### US-1.3 — Parser la section PATIENT2 (0x0113) pour le genre

**En tant que** converter,  
**je veux** extraire le genre depuis PATIENT2,  
**afin de** renseigner `administrativeGenderCode` dans le XML FDA.

**Encoding confirmé :** pattern `02 01 XX` à PATIENT2 data +0x7C
- 0x00 = NotSet, 0x01 = Male, 0x02 = Female, 0x03 = Unknown

**Critères d'acceptation :**
- [ ] Chercher le pattern `0x02 0x01` dans PATIENT2 et lire le byte suivant
- [ ] Mapper vers `types.GenderCode`
- [ ] Valeur par défaut = Unknown si pattern absent

### US-1.4 — Parser la section MEASUREMENT (0x0007)

**En tant que** converter,  
**je veux** extraire les valeurs analytiques ECG (HR, PR, QRS, QT, axes, amplitudes),  
**afin de** renseigner les `controlVariable` dans le XML FDA.

**Layout confirmé (i16/u16 Big-Endian) :**
```
+0x08  u16  HeartRate (bpm)
+0x0A  u16  PRInterval (ms)
+0x0C  u16  QRSDuration (ms)
+0x0E  u16  QTInterval (ms)
+0x10  u16  QTcInterval (ms)
+0x12  i16  PAxis (deg)
+0x14  i16  QRSAxis (deg)
+0x16  i16  TAxis (deg)
+0x18  u16  V5RAmplitude (nV → ÷1000 pour µV)
+0x1A  u16  V1SAmplitude (nV → ÷1000 pour µV)
```

**Critères d'acceptation :**
- [ ] Lire tous les champs en Big-Endian
- [ ] Convertir amplitudes en µV (÷ 1000)
- [ ] Valeur 0x8000 ou 0xFFFF = champ non renseigné → skip

### US-1.5 — Parser la section RECORD (0x0008)

**En tant que** converter,  
**je veux** extraire les paramètres d'enregistrement,  
**afin de** configurer le décodeur waveform.

**Layout :**
```
+0x00  u16 BE  SampleRate (ex: 500 Hz)
+0x02  u16 BE  BytesPerSample (= 2)
+0x04  u16 BE  CompressionType (doit être 16)
+0x1C4 u16 BE  TotalSamples (ex: 5000)
```

**Critères d'acceptation :**
- [ ] Validation : compression_type == 16 (sinon erreur non supporté)
- [ ] Extraire sample_rate et total_samples
- [ ] Retourner les données brutes de la section pour le décodeur

### US-1.6 — Parser la section SYSTEM (0x0001)

**En tant que** converter,  
**je veux** extraire le modèle de l'appareil,  
**afin de** renseigner `ManufacturerModelName` dans la série.

**Layout :** `data +0x00` = string ASCII (ex: "01002350K")

**Critères d'acceptation :**
- [ ] Extraire le string modèle, trimmer
- [ ] Format attendu : "XXXXXXXXXK" où X = chiffres (ex: "01002350K" → modèle "2350K")

### US-1.7 — Flag --metadata-json

**En tant qu'utilisateur**,  
**je veux** obtenir les métadonnées patient en JSON sans conversion complète,  
**afin de** vérifier/enrichir les données avant la conversion.

**Critères d'acceptation :**
- [ ] Flag `--metadata-json` : sort un JSON sur stdout et quitte
- [ ] JSON inclut : patientID, familyName, givenName, gender, location, datetime, deviceModel
- [ ] JSON inclut toutes les mesures MEASUREMENT
- [ ] Exit code 0 même si waveform non disponible

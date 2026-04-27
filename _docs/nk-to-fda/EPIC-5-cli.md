# EPIC-5 — CLI Integration

## Objectif

Créer une commande `nk-to-fda` avec cobra, cohérente avec les autres converters du projet.

## User Stories

### US-5.1 — Commande principale

**Usage :**
```bash
nk-to-fda --input <file.DAT> [--output <file.xml>] [--debug]
```

**Critères d'acceptation :**
- [ ] Flag `--input` / `-i` : obligatoire, chemin du fichier `.DAT`
- [ ] Flag `--output` / `-o` : optionnel, défaut = stdout
- [ ] Flag `--debug` : affiche les métadonnées extraites sur stderr
- [ ] Si `--output` absent : écrire le XML sur stdout (comme les autres converters)
- [ ] Exit code 1 + message d'erreur sur stderr en cas d'échec

### US-5.2 — Flag --metadata-json

**Usage :**
```bash
nk-to-fda --input <file.DAT> --metadata-json
```

**Critères d'acceptation :**
- [ ] Sort un JSON sur stdout avec toutes les métadonnées extraites
- [ ] Ne décode PAS les waveforms (rapide)
- [ ] Champs JSON :
  ```json
  {
    "patientID": "250606392",
    "familyName": "feugueur",
    "givenName": "feugueur",
    "gender": "F",
    "location": "1453 - 3E EST (20177805)",
    "datetime": "20250910135412",
    "deviceModel": "2350K",
    "heartRate": 105,
    "prInterval": 260,
    "qrsDuration": 94,
    "qtInterval": 374,
    "qtcInterval": 435,
    "pAxis": 28,
    "qrsAxis": -28,
    "tAxis": 183,
    "v5rAmplitude": 1.29,
    "v1sAmplitude": 0.55,
    "sampleRate": 500,
    "totalSamples": 5000
  }
  ```
- [ ] Exit code 0

### US-5.3 — Structure cmd/nk-to-fda/main.go

**Critères d'acceptation :**
- [ ] Package main avec cobra rootCmd
- [ ] Imports uniquement de la stdlib + cobra + package local nktofda
- [ ] Cohérent avec cmd/philips-to-fda/main.go

# EPIC-2 — Waveform Decoder (Type 16)

## Objectif

Porter le décodeur Python `nk_decoder_v6.py` en Go pour décompresser les 8 leads mesurés
(I, II, V1–V6) depuis la section RECORD du fichier `.DAT`.

## Référence

- Source Python : `data_nk/nk_decoder_v6.py` + `mode_handlers.py`
- Validation : 100% exact vs `250606392_20250910135412.FDA.xml` (5000/5000 samples par lead)
- Algorithme : Huffman VLC per-frame + cumsum global + mode handlers

## Architecture du décodeur

```
DecodeFile(dat []byte) → map[LeadName][]int32

  ┌─ parse_code_table(rec, ct_off) → huffSym[33]
  ├─ extract_modes_params(rec, 0x0AA2) → n_segs, modes[], params[]
  ├─ decode_lead_frame(rec, ct_off, data_off, ..., lead_idx) → []int32
  │    ├─ decode_subunit × n_segs → all_raw[]
  │    ├─ global cumsum(all_raw) → buf[]
  │    └─ for each seg: seg_vals = buf[k] << 3; mode_handler(...)
  └─ repeat for leads I, II, V1–V6
```

## Frame layout dans RECORD data (offset fichier 0x1AA6)

| Frame | Offset RECORD data | Lead | Rôle |
|-------|--------------------|------|------|
| F0 (AVG) | 0x01C6 | — | Template AVG (modes 2/3, non requis pour mode 0/1) |
| Gap | 0x0AA2 | I | Flags mode/param + Lead I |
| F1 | 0x1A66 | II | Lead II |
| F2 | 0x29E4 | V1 | Lead V1 |
| F3 | 0x359A | V2 | Lead V2 |
| F4 | 0x417A | V3 | Lead V3 |
| F5 | 0x4D5A | V4 | Lead V4 |
| F6 | 0x594A | V5 | Lead V5 |
| F7 | 0x6560 | V6 | Lead V6 |

## User Stories

### US-2.1 — Huffman code table parser

**Critères d'acceptation :**
- [ ] Parser 33 entrées : `u8 bit_length` + `ceil(bl/8)` octets codeword (MSB-aligned sur 32 bits)
- [ ] Entrée `bit_length == 0` → symbole inutilisé (skip)

**Format :**
```
u16 BE  total_size
33 × { u8 bit_len; if bit_len > 0: ceil(bit_len/8) bytes codeword }
```

### US-2.2 — Decode sub-unit (Huffman VLC)

**Critères d'acceptation :**
- [ ] Sub-unit = `u32 BE total_bits` + bits compressés
- [ ] Décodage MSB-first via `read_window32 + mask comparison`
- [ ] Tables DELTA_TABLE et EXTRA_BITS_COUNT (33 entrées) portées de Python
- [ ] Extra bits lus après le symbole pour les entrées 26–32
- [ ] `total_bits == 0` → sub-unit vide, retourner immédiatement
- [ ] Alignement word-aligned (arrondir à l'octet pair) entre sub-units

**Constantes :**
```go
var deltaTable = [33]int{0, -1, 1, -2, 2, -3, 3, -4, 4, -5, 5, -6, 6, -7, 7, -8, 8,
    -9, 9, -10, 10, -11, 11, -12, 12, -13, 13, -29, 29, -45, 45, -301, 0}
var extraBitsCount = [33]int{0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,4,4,4,4,8,8,16}
```

### US-2.3 — Extraction flags mode/param (Gap frame)

**Critères d'acceptation :**
- [ ] `bit_count = u16 BE at section_start + 2`
- [ ] `n_segments = bit_count >> 2`
- [ ] 7 mots de flags packés (au_898[2..8]) extraits depuis `section_start + i*2` pour i=2..8
- [ ] Formule de dépaquetage identique à `extract_modes_params` Python

### US-2.4 — Mode 0: simple write (fun_100031b8)

**Critères d'acceptation :**
- [ ] Écrire `count` samples dans `output[t*8 + lead]` avec truncation i16
- [ ] Retourner `pos + count` ou overflow si `pos + count > max_end`

### US-2.5 — Mode 1: 2× upsampling (fun_10003215)

**Critères d'acceptation :**
- [ ] Si count < 5 : écriture simple (comme mode 0)
- [ ] Si count ≥ 5 : upsampling avec interpolation midpoint entre paires
- [ ] Paramètre `param` (0 ou 1) contrôle le décalage final de 1 sample
- [ ] Retourner `uVar5 + 4`

### US-2.6 — Decode lead frame complet

**Critères d'acceptation :**
- [ ] Pour Gap frame : `ct_off = 0x0AB6`, `data_off = ct_off + 2 + ct_size`
- [ ] Pour F1–F7 : `ct_off = frame_start + 2`, `data_off = ct_off + 2 + ct_size`
- [ ] Pipeline : decode_subunit × n_segs → all_raw → global cumsum → << 3 → mode_handler
- [ ] Output : `[]int32` de longueur `n_samples` (5000)

### US-2.7 — Validation exacte vs FDA XML

**Critères d'acceptation :**
- [ ] Test d'intégration : décoder `00000005.DAT` → comparer avec `FDA.xml`
- [ ] 100% exact pour les 8 leads (5000/5000 samples)

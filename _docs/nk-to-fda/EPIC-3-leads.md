# EPIC-3 — 12-Lead Derivation

## Objectif

Calculer les 4 leads dérivés standard (III, aVR, aVL, aVF) à partir des 8 leads mesurés,
pour obtenir un ECG 12 dérivations complet.

## Formules (Einthoven + Wilson augmenté)

```
III  = II  - I
aVR  = -(I + II) / 2
aVL  = (2·I  - II) / 2
aVF  = (2·II - I)  / 2
```

**Validation :** `III = II - I` confirmée sample-par-sample sur `00000005.DAT` vs FDA XML.

## User Stories

### US-3.1 — Dériver les 4 leads augmentés

**En tant que** converter,  
**je veux** calculer III, aVR, aVL, aVF à partir de I et II (integer math),  
**afin d'** obtenir les 12 leads pour le FDA XML.

**Critères d'acceptation :**
- [ ] Division entière (truncation) — acceptable car samples sont multiples de 8
- [ ] Longueur des slices = len(I) = len(II) = 5000
- [ ] Pas de saturation i16 requise (valeurs restent dans la plage valide)

### US-3.2 — Ordre des leads pour le builder

**En tant que** converter,  
**je veux** que les leads soient dans l'ordre FDA standard,  
**afin que** les séquences dans le XML soient dans le bon ordre.

**Ordre FDA (confirmé sur FDA XML de référence) :**
```
I, II, V1, V2, V3, V4, V5, V6, III, aVR, aVL, aVF
```

**Critères d'acceptation :**
- [ ] La map passée au builder inclut les 12 LeadCode
- [ ] L'ordre des sections dans le XML reproduit l'ordre de référence

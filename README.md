# converter-fda

A collection of Go modules for converting ECG binary formats to and from FDA-compliant HL7 v3 aECG XML, using the [hl7v3-aecg](https://github.com/LIRYC-IHU/hl7v3-aecg) library.

## Overview

Medical ECG data exists in a variety of proprietary and standard binary formats. This project provides standalone converters to produce (or consume) FDA-compliant HL7 v3 Annotated ECG (aECG) XML files, as required for clinical trial submissions.

Each converter lives in its own directory and can be used independently.

## Converters

| Converter           | Source format         | Target format         | Status |
| ------------------- | --------------------- | --------------------- | ------ |
| `nk-to-fda`         | NK .DAT (PEC format)  | FDA HL7 v3 aECG XML   | Done   |
| `nk-to-dicom`       | NK .DAT (PEC format)  | DICOM (ECG waveforms) | Done   |
| `dicom-to-fda`      | DICOM (ECG waveforms) | FDA HL7 v3 aECG XML   | Done   |
| `fda-to-dicom`      | FDA HL7 v3 aECG XML   | DICOM (ECG waveforms) | Done   |
| `philipsXml-to-fda` | Philips ECG XML       | FDA HL7 v3 aECG XML   | Done   |
| `mindray-to-fda`    | BeneHeart R12 .PAT    | FDA HL7 v3 aECG XML   | Done   |
| `mindray-to-dicom`  | BeneHeart R12 .PAT    | DICOM (ECG waveforms) | Done   |

## Project Structure

```
converter-fda/
├── nk-to-fda/          # NK .DAT (PEC) → FDA HL7 v3 aECG XML
├── nk-to-dicom/        # NK .DAT (PEC) → DICOM ECG
├── dicom-to-fda/       # DICOM → FDA HL7 v3 aECG XML
├── fda-to-dicom/       # FDA HL7 v3 aECG XML → DICOM
├── philipsXml-to-fda/  # Philips XML → FDA HL7 v3 aECG XML
├── mindray-to-fda/  # Mindray .PAT → FDA HL7 v3 aECG XML
├── mindray-to-dicom/  # Mindray .PAT → DICOM ECG
└── go.mod
```

<!-- CLI_TOOLS_START -->
## CLI Tools

- `fda-to-dicom` 
```bash
go install github.com/LIRYC-IHU/ecg-bridge/cmd/fda-to-dicom@latest
```

- `mindray-to-dicom` 
```bash
go install github.com/LIRYC-IHU/ecg-bridge/cmd/mindray-to-dicom@latest
```

- `mindray-to-fda` 
```bash
go install github.com/LIRYC-IHU/ecg-bridge/cmd/mindray-to-fda@latest
```

- `muse-to-dicom` 
```bash
go install github.com/LIRYC-IHU/ecg-bridge/cmd/muse-to-dicom@latest
```

- `muse-to-fda` 
```bash
go install github.com/LIRYC-IHU/ecg-bridge/cmd/muse-to-fda@latest
```

- `nk-to-fda` 
```bash
go install github.com/LIRYC-IHU/ecg-bridge/cmd/nk-to-fda@latest
```

- `philips-to-dicom` 
```bash
go install github.com/LIRYC-IHU/ecg-bridge/cmd/philips-to-dicom@latest
```

- `philips-to-fda` 
```bash
go install github.com/LIRYC-IHU/ecg-bridge/cmd/philips-to-fda@latest
```

<!-- CLI_TOOLS_END -->

## Dependencies

- **[hl7v3-aecg](https://github.com/LIRYC-IHU/hl7v3-aecg)** — Go library for generating/parsing FDA-compliant HL7 v3 aECG XML files (12-lead ECG, annotations, clinical trial metadata)

```bash
go get github.com/LIRYC-IHU/hl7v3-aecg
```

## Requirements

- Go 1.20+

## References

- [HL7 v3 aECG Specification (March 2005)](https://www.hl7.org/documentcenter/public/standards/v3/V3_DT_SPEC_AECG_R1_2005MAR.zip)
- [FDA ECG Warehouse](https://www.fda.gov/science-research/bioinformatics-tools/ecg-warehouse)
- [DICOM Standard — Waveform Module](https://dicom.nema.org/medical/dicom/current/output/chtml/part03/sect_C.10.html)

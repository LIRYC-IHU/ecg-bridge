# converter-fda

A collection of Go modules for converting ECG binary formats to and from FDA-compliant HL7 v3 aECG XML, using the [hl7v3-aecg](https://github.com/LIRYC-IHU/hl7v3-aecg) library.

## Overview

Medical ECG data exists in a variety of proprietary and standard binary formats. This project provides standalone converters to produce (or consume) FDA-compliant HL7 v3 Annotated ECG (aECG) XML files, as required for clinical trial submissions.

Each converter lives in its own directory and can be used independently.

## Converters

| Converter           | Source format         | Target format         | Status  |
| ------------------- | --------------------- | --------------------- | ------- |
| `dicom-to-fda`      | DICOM (ECG waveforms) | FDA HL7 v3 aECG XML   | Planned |
| `fda-to-dicom`      | FDA HL7 v3 aECG XML   | DICOM (ECG waveforms) | Planned |
| `philipsXml-to-fda` | Philips ECG XML       | FDA HL7 v3 aECG XML   | Planned |

## Project Structure

```
converter-fda/
├── dicom-to-fda/       # DICOM → FDA HL7 v3 aECG XML
├── fda-to-dicom/       # FDA HL7 v3 aECG XML → DICOM
├── philipsXml-to-fda/  # Philips XML → FDA HL7 v3 aECG XML
└── go.mod
```

<!-- CLI_TOOLS_START -->
<!-- CLI_TOOLS_END -->

## Dependencies

- **[hl7v3-aecg](https://github.com/LIRYC-IHU/hl7v3-aecg)** — Go library for generating/parsing FDA-compliant HL7 v3 aECG XML files (12-lead ECG, annotations, clinical trial metadata)

```bash
go get github.com/LIRYC-IHU/hl7v3-aecg
```

## Requirements

- Go 1.20+

## Branch Strategy

Each converter is developed on its own branch:

```
dicom-to-fda    # DICOM → FDA converter
fda-to-dicom    # FDA → DICOM converter
philipsXml-to-fda # Philips XML → FDA converter
```

## References

- [HL7 v3 aECG Specification (March 2005)](https://www.hl7.org/documentcenter/public/standards/v3/V3_DT_SPEC_AECG_R1_2005MAR.zip)
- [FDA ECG Warehouse](https://www.fda.gov/science-research/bioinformatics-tools/ecg-warehouse)
- [DICOM Standard — Waveform Module](https://dicom.nema.org/medical/dicom/current/output/chtml/part03/sect_C.10.html)

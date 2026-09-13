# IHE Cardiology — Retrieve ECG for Display: what is and is not implemented

The project states in its regulatory filing that it applies the requirements of
transaction **CARD-6**. That statement is only defensible if it is precise about
which half of the profile it means, because the profile has two independent
halves and this repository implements one of them.

Reference: IHE Cardiology Technical Framework, Volume 2 (Transactions),
Revision 4.0, Final Text, 2011-08-05 — §4.5 and §4.6. The profile itself is
**Final Text**, unlike the Resting ECG Workflow supplement, which has been in
Trial Implementation since 2011 and covers the identity workflow.

> **Summary.** The document-content requirements of CARD-6 (§4.6.4.2.2) are
> implemented and each is pinned by a test. The CARD-5 and CARD-6 **transactions**
> are not implemented. This repository does not claim the Information Source
> actor of the Retrieve ECG for Display profile, and no IHE Integration Statement
> is issued.

## Half one — document content (§4.6.4.2.2). Implemented.

These are requirements on the ECG document itself, independent of how it is
retrieved. `ecgpdf/render_test.go` asserts each against the rendered PDF.

| § | Requirement | Status |
|---|---|---|
| 4.6.4.2.2.1 | Patient name **and** ID; no anonymous documents | Implemented — see caveats |
| 4.6.4.2.2.2 | Date and time of recording | Implemented |
| 4.6.4.2.2.3 | Diagnostic quality (ANSI/AAMI EC-11 as equivalence target) | Argued, not measurable — no standard exists for softcopy |
| 4.6.4.2.2.4 | Calibration pulse, 1 mV high × 200 ms wide | Implemented |
| | Nominal 1 mm grid | Implemented — exactly 1 mm at 25 mm/s · 10 mm/mV |
| | Fixed aspect ratio; squares never rectangles | Implemented and tested |
| | Major axes every 5 mm, darker or thicker | Implemented |
| | Minor axes at 1 mm, fainter, thinner or dotted | Implemented |
| | mm/s and mm/mV statements (*recommended, optional*) | Implemented |
| | Per-lead gain determinable when leads differ | Satisfied — every lead is drawn at one gain (see below) |
| | Lead labels | Implemented |
| | Indication denoting each lead-to-lead transition | Implemented |
| | Statement of frequency content | Implemented, and its absence stated explicitly |
| 4.6.4.2.2.5 | Confirmed / unconfirmed interpretation statement | Implemented, and its absence stated explicitly |
| 4.6.4.2.2.6 | `Application/pdf` mandatory | Implemented |
| | Vector drawing commands, not a rasterised image | Implemented and tested (no image XObject, no `Do`) |
| | `Image/svg+xml` (*optional*) | **Not implemented** |

### Per-lead gains — and a correction

§4.6.4.2.2.4 requires that when leads carry different gains — precordial leads
at half gain is the case the profile names — the document give enough
information to determine each lead's gain.

**This requirement is satisfied trivially**, and an earlier version of this file
said otherwise. The clause is about the *rendered* gain: leads drawn at
different mm/mV on the page. This renderer draws every lead at the same
10 mm/mV, stated on the document, so there is never a per-lead gain to declare.

What was conflated with it is a separate and more serious matter: a **fidelity
bug**, not a conformance gap. aECG and MUSE state the *digitisation* scale
(µV/LSB) per lead, and both parsers kept the first lead's value and discarded
the rest. A recording whose leads were digitised differently therefore had the
others' amplitudes silently halved or doubled.

The two are easy to confuse and worth separating: applying each lead's own
µV/LSB is precisely what allows every lead to be drawn at the same mm/mV.

- **aECG read path: fixed.** Per-lead scales are carried from
  `parseSequenceSet` through `FDAData.LeadSensitivity` and
  `ecgpdf.Report.ScaleUVByLead` to the renderer, and to the DICOM builder's
  normalisation. `TestPerLeadScaleRendersTheSameVoltageIdentically` asserts the
  property that matters — the same voltage expressed at a different
  digitisation scale renders identically — with a companion test proving that
  comparison is sensitive to amplitude at all.
- **MUSE write path: still refused** (`ErrPerLeadScale`). The aECG writer used
  here takes one scale per series and cannot express more, so the conversion
  genuinely cannot represent such a recording. Lifting it needs a per-lead
  scale in `hl7v3-aecg`.

Note what was **not** done: renormalising the samples in the parser onto a
common scale. It is the obvious parser-local fix and it would modify sample
values in the decode layer, with rounding, destroying the bit-exactness the
regulatory position rests on.

### Anonymous documents

§4.6.4.2.2.1 is explicit: *"The Cardiology Technical Framework does not support
the delivery of anonymous ECG documents in this transaction."*

ecg-hub can produce an anonymised PDF (`?anonymize=1`) for research export. Such
a document is **outside** this transaction and must not be presented as
CARD-6 conformant. The same applies to any document whose patient name was never
resolved: a conformant document needs the name, which on an identifier-only
acquisition can only come from the HIS.

## Half two — the transactions (§4.5, §4.6). Not implemented.

Neither transaction exists in this repository or in ecg-hub. To claim the
Information Source actor, the following would be required.

### CARD-6 — Retrieve ECG Document for Display (§4.6.4.1.2)

HTTP GET binding at an implementation-independent operation location:

```
http://<location>/IHERetrieveDocument?requestType=DOCUMENT&documentUID=<OID|UUID>&preferredContentType=application%2fpdf
```

| Parameter | Req | Values |
|---|---|---|
| `requestType` | R | `DOCUMENT` |
| `documentUID` | R | OID (ITI-TF-2 Appendix B) **or** UUID (DCE 1.1 RPC) |
| `preferredContentType` | R | `Application/pdf`, or `Image/svg+xml` |

HTTP fields: `Expires` is **required** on the response and may not exceed one
week; `Accept` and `Accept-Language` are optional on the request, and the
response must not contradict `Accept`. Redirects 301/302/303/307 are permitted.

### CARD-5 — Retrieve ECG List (§4.5)

Derived from Retrieve Specific Information for Display [ITI-11], with
`requestType=SUMMARY-CARDIOLOGY-ECG`. The response is XML per CARD TF-2
Appendix C, referencing a server-side stylesheet so a Display actor can render
it without processing, with `Expires` set to zero so it is never cached. Each
entry must carry a hyperlink formatted as a CARD-6 request.

The profile additionally requires an Information Source supporting CARD-5 to
support the `SUMMARY` and `SUMMARY-CARDIOLOGY` options of ITI-11.

### What exists instead

`GET /api/v1/ecgs/:id/download?format=pdf` on ecg-hub: an authenticated route
(bearer token or API key) with its own parameter names and its own identifiers.
It serves the same document to a caller who is already authorised, and it is not
the transaction — different URL shape, different parameter names, no `documentUID`,
no `preferredContentType`, no `Expires` contract, and an authentication model the
profile does not describe.

That last point is not incidental. CARD-6 predates modern API authentication and
expects a Display actor to follow a plain URL, so exposing it means deciding how
access is controlled — a reverse proxy, a token in the URL, or network
restriction. That decision is unmade, which is one reason the transaction is not
simply switched on.

## Why this file exists

An IHE Integration Statement with an actor/transaction matrix, including explicit
"not supported" entries, is on the regulatory-readiness backlog. This is the
interim version of it. The distinction it records — content requirements applied,
transactions not implemented — is what keeps the statement in the ANSM filing
accurate.

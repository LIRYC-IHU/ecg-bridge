package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	nktofda "converter-fda/nk-to-fda"

	"github.com/go-pdf/fpdf"
)

// Standard clinical ECG print constants.
const (
	mmPerSec = 25.0 // paper speed
	mmPerMV  = 10.0 // gain
)

// Page geometry (A4 landscape, mm).
const (
	margin  = 8.0
	pageW   = 297.0
	rightX  = 200.0 // left edge of the top-right statements column
	gridX   = margin
	gridW   = 250.0 // 4 columns * 62.5mm == 10s at 25mm/s
	gridTop = 74.0
	rowH    = 28.0
	rhythmH = 30.0
)

// lbl holds the active language's static labels. Set once at the top of
// renderPDF; rendering is single-shot and single-threaded.
var lbl labels

// renderPDF lays out the NK-style 12-lead report on a single A4 landscape page
// and writes the PDF bytes to w. The creation/modification dates are pinned to
// the recording datetime (or a fixed epoch when unknown) so the document
// metadata is reproducible rather than wall-clock dependent.
func renderPDF(nd *nktofda.NKData, lang string, w io.Writer) error {
	lbl = labelsFor(lang)
	scaleUV := nd.Record.Scale
	if scaleUV == 0 {
		scaleUV = 1.25 // NK type-16 default µV/digit
	}
	sr := float64(nd.Record.SampleRate)
	if sr == 0 {
		sr = 500
	}

	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetMargins(0, 0, 0)
	// Pin the document dates for deterministic output (stable SHA256).
	docDate := nd.Patient.RecordingAt
	if docDate.IsZero() {
		docDate = time.Unix(0, 0).UTC()
	}
	pdf.SetCreationDate(docDate)
	pdf.SetModificationDate(docDate)
	pdf.AddPage()
	tr := pdf.UnicodeTranslatorFromDescriptor("") // cp1252 for accented French

	drawPatientBlock(pdf, tr, nd)
	drawStatements(pdf, tr, nd, lang)
	drawMeasurements(pdf, tr, nd)
	drawScaleNote(pdf, tr)

	drawGrid(pdf, gridX, gridTop, gridW, 3*rowH+rhythmH)
	drawSignals(pdf, nd, sr, scaleUV)

	drawFooter(pdf, tr, nd)

	return pdf.Output(w)
}

// --- header: patient identity (top-left) ---

func drawPatientBlock(pdf *fpdf.Fpdf, tr func(string) string, nd *nktofda.NKData) {
	p := nd.Patient
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "", 8.5)

	const lh = 4.4
	y := 7.0
	put := func(x, y float64, s string) {
		pdf.SetXY(x, y)
		pdf.Write(lh, tr(s))
	}

	name := strings.TrimSpace(p.FamilyName + " " + p.GivenName)
	put(margin, y, lbl.id+" "+dash(p.PatientID))
	y += lh
	put(margin, y, lbl.name+" "+dash(name))
	y += lh
	put(margin, y, lbl.sex+" "+dash(p.Gender))
	put(70, y, lbl.dob+" "+fmtDOB(p.BirthDate))
	put(125, y, withSuffix(p.Age, lbl.ageSuffix))
	y += lh
	put(margin, y, withSuffix(p.Height, lbl.cmSuffix))
	put(40, y, withSuffix(p.Weight, lbl.kgSuffix))
	put(70, y, lbl.bp)
	y += lh
	put(margin, y, lbl.meds+" "+dash(strings.Join(p.Medications, ", ")))
	y += lh
	put(margin, y, lbl.symptoms+" "+dash(p.Symptoms))
	y += lh
	put(margin, y, lbl.history+" "+dash(p.History))
}

// --- header: interpretive statements with codes (top-right) ---

func drawStatements(pdf *fpdf.Fpdf, tr func(string) string, nd *nktofda.NKData, lang string) {
	pdf.SetTextColor(0, 0, 0)
	const lh = 4.4
	y := 7.0
	for _, st := range nd.Statements {
		txt := nktofda.StatementText(lang, st.Code)
		if txt == "" {
			continue
		}
		if st.Overall {
			pdf.SetFont("Helvetica", "B", 8.5)
		} else {
			pdf.SetFont("Helvetica", "", 8.5)
		}
		pdf.SetXY(rightX, y)
		pdf.Write(lh, tr(st.Code))
		pdf.SetXY(rightX+14, y)
		pdf.Write(lh, tr(txt))
		y += lh
	}
}

// --- header: measurements table (label / right-aligned value / unit) ---

func drawMeasurements(pdf *fpdf.Fpdf, tr func(string) string, nd *nktofda.NKData) {
	m := nd.Measurement
	pdf.SetFont("Helvetica", "", 8.5)
	pdf.SetTextColor(0, 0, 0)

	y := 40.0
	const lh = 4.3
	row := func(label, value, unit string) {
		pdf.SetXY(margin, y)
		pdf.CellFormat(46, lh, tr(label), "", 0, "L", false, 0, "")
		pdf.SetXY(54, y)
		pdf.CellFormat(24, lh, tr(value), "", 0, "R", false, 0, "")
		pdf.SetXY(80, y)
		pdf.CellFormat(16, lh, tr(unit), "", 0, "L", false, 0, "")
		y += lh
	}

	row(lbl.hr, fmt.Sprintf("%d", m.HeartRate), "bpm")
	row(lbl.prInt, fmt.Sprintf("%d", m.PRInterval), "ms")
	row(lbl.qrsDur, fmt.Sprintf("%d", m.QRSDuration), "ms")
	row(lbl.qtQtc, fmt.Sprintf("%d / %d", m.QTInterval, m.QTcInterval), "ms")
	row(lbl.axis, fmt.Sprintf("%d / %d / %d", m.PAxis, m.QRSAxis, m.TAxis), "°")
	row(lbl.amplDiv, fmt.Sprintf("%.3f / %.3f", m.V5RAmplitude, m.V1SAmplitude), "mV")
	row(lbl.amplSum, fmt.Sprintf("%.3f", m.V5RAmplitude+m.V1SAmplitude), "mV")
}

// --- header: scale / filter note above the signals ---

func drawScaleNote(pdf *fpdf.Fpdf, tr func(string) string) {
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetXY(40, gridTop-4)
	pdf.Write(4, tr(lbl.scaleNote))
	pdf.SetXY(rightX+30, gridTop-4)
	pdf.Write(4, tr("10 mm/mV"))
}

// --- signals ---

func drawSignals(pdf *fpdf.Fpdf, nd *nktofda.NKData, sr, scaleUV float64) {
	I := nd.Leads["I"]
	II := nd.Leads["II"]
	III, aVR, aVL, aVF := nktofda.DeriveLeads(I, II)
	leadData := map[string][]int32{
		"I": I, "II": II, "III": III,
		"aVR": aVR, "aVL": aVL, "aVF": aVF,
		"V1": nd.Leads["V1"], "V2": nd.Leads["V2"], "V3": nd.Leads["V3"],
		"V4": nd.Leads["V4"], "V5": nd.Leads["V5"], "V6": nd.Leads["V6"],
	}
	layout := [3][4]string{
		{"I", "aVR", "V1", "V4"},
		{"II", "aVL", "V2", "V5"},
		{"III", "aVF", "V3", "V6"},
	}
	colW := gridW / 4.0
	secPerCol := colW / mmPerSec // 2.5s

	// Round caps/joins keep the trace smooth at any zoom level.
	pdf.SetLineCapStyle("round")
	pdf.SetLineJoinStyle("round")

	for r := 0; r < 3; r++ {
		cellTop := gridTop + float64(r)*rowH
		for c := 0; c < 4; c++ {
			name := layout[r][c]
			x0 := gridX + float64(c)*colW
			startSec := float64(c) * secPerCol
			plotLead(pdf, leadData[name], sr, scaleUV, x0, cellTop, colW, rowH, startSec, secPerCol)
			leadLabel(pdf, name, x0+1, cellTop+1)
		}
	}

	// Rhythm strip: lead II across the full 10s.
	rhTop := gridTop + 3*rowH
	plotLead(pdf, II, sr, scaleUV, gridX, rhTop, gridW, rhythmH, 0, gridW/mmPerSec)
	leadLabel(pdf, "II", gridX+1, rhTop+1)
}

// plotLead draws one lead trace as a single full-resolution vector path,
// clipped to its cell rectangle. Clipping hides any overflow cleanly without
// distorting tall QRS complexes into flat plateaus (the old amplitude clamp),
// so the trace stays faithful when zoomed.
func plotLead(pdf *fpdf.Fpdf, samples []int32, sr, scaleUV, cellX, cellTop, cellW, cellH, startSec, durSec float64) {
	if len(samples) == 0 {
		return
	}
	startIdx := int(startSec * sr)
	endIdx := int((startSec + durSec) * sr)
	if endIdx > len(samples) {
		endIdx = len(samples)
	}
	if startIdx >= endIdx {
		return
	}
	baseY := cellTop + cellH/2

	pdf.ClipRect(cellX, cellTop, cellW, cellH, false)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetLineWidth(0.2)
	for i := startIdx; i < endIdx; i++ {
		x := cellX + float64(i-startIdx)/sr*mmPerSec
		mv := float64(samples[i]) * scaleUV / 1000.0
		y := baseY - mv*mmPerMV
		if i == startIdx {
			pdf.MoveTo(x, y)
		} else {
			pdf.LineTo(x, y)
		}
	}
	pdf.DrawPath("D")
	pdf.ClipEnd()
}

// drawGrid paints the red millimetric ECG graph paper (1mm minor, 5mm major).
func drawGrid(pdf *fpdf.Fpdf, x, y, w, h float64) {
	pdf.SetLineWidth(0.1)
	pdf.SetDrawColor(255, 200, 200)
	for gx := 0.0; gx <= w+0.01; gx += 1 {
		pdf.Line(x+gx, y, x+gx, y+h)
	}
	for gy := 0.0; gy <= h+0.01; gy += 1 {
		pdf.Line(x, y+gy, x+w, y+gy)
	}
	pdf.SetLineWidth(0.3)
	pdf.SetDrawColor(255, 130, 130)
	for gx := 0.0; gx <= w+0.01; gx += 5 {
		pdf.Line(x+gx, y, x+gx, y+h)
	}
	for gy := 0.0; gy <= h+0.01; gy += 5 {
		pdf.Line(x, y+gy, x+w, y+gy)
	}
}

func leadLabel(pdf *fpdf.Fpdf, name string, x, y float64) {
	pdf.SetFont("Helvetica", "B", 7)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetXY(x, y)
	pdf.Write(3, name)
}

// --- footer ---

func drawFooter(pdf *fpdf.Fpdf, tr func(string) string, nd *nktofda.NKData) {
	p := nd.Patient
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(0, 0, 0)
	y := gridTop + 3*rowH + rhythmH + 4

	left := fmt.Sprintf("%s   %s %s", dash(p.DeviceModel), lbl.service, dash(p.Department))
	pdf.SetXY(margin, y)
	pdf.Write(4, tr(left))

	right := fmt.Sprintf("%s %s, %s", lbl.exam, dash(p.Operator), dash(p.Location))
	pdf.SetXY(rightX, y)
	pdf.Write(4, tr(right))
}

// --- helpers ---

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func withSuffix(v, suffix string) string {
	if strings.TrimSpace(v) == "" {
		return "—" + suffix
	}
	return v + suffix
}

// fmtDOB renders a YYYYMMDD birth date as YYYY-MM-DD, or — when unknown.
func fmtDOB(b string) string {
	if len(b) != 8 {
		return "—"
	}
	return b[:4] + "-" + b[4:6] + "-" + b[6:8]
}

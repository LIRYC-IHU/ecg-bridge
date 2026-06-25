package ecgpdf

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

// ECG print scale. Enlarged from the 25mm/s · 10mm/mV baseline to use the
// available right/bottom space; the red grid below is derived from these so it
// stays a valid 0.2s / 0.5mV measurement grid at this scale.
const (
	mmPerSec = 28.0 // paper speed
	mmPerMV  = 12.0 // gain
)

// Page geometry (A4 landscape, mm).
const (
	margin  = 8.0
	rightX  = 200.0 // left edge of the top-right statements column
	gridX   = margin
	gridW   = 280.0 // 4 columns * 70mm == 10s at 28mm/s
	gridTop = 74.0
	rowH    = 29.0
	rhythmH = 34.0
)

// lbl holds the active language's static labels. Set once at the top of Render;
// rendering is single-shot and single-threaded.
var lbl labels

// Render lays out a 12-lead report on a single A4 landscape page and writes the
// PDF bytes to w. The creation/modification dates are pinned to the recording
// datetime (or a fixed epoch when unknown) so document metadata is reproducible
// rather than wall-clock dependent.
func Render(r *Report, lang string, w io.Writer) error {
	lbl = labelsFor(lang)
	scaleUV := r.ScaleUV
	if scaleUV == 0 {
		scaleUV = 1.25
	}
	sr := r.SampleRate
	if sr == 0 {
		sr = 500
	}

	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetMargins(0, 0, 0)
	docDate := r.RecordingAt
	if docDate.IsZero() {
		docDate = time.Unix(0, 0).UTC()
	}
	pdf.SetCreationDate(docDate)
	pdf.SetModificationDate(docDate)
	pdf.AddPage()
	tr := pdf.UnicodeTranslatorFromDescriptor("") // cp1252 for accented French

	drawPatientBlock(pdf, tr, r)
	drawStatements(pdf, tr, r)
	drawMeasurements(pdf, tr, r)
	drawPhysicianBlock(pdf, tr)
	drawScaleNote(pdf, tr, r)

	drawGrid(pdf, gridX, gridTop, gridW, 3*rowH+rhythmH)
	drawSignals(pdf, r, sr, scaleUV)

	drawFooter(pdf, tr, r)
	drawDisclaimer(pdf, tr)

	return pdf.Output(w)
}

// Output writes the PDF bytes to outPath, or—when outPath is empty—prints them
// base64-encoded to stdout so external tools can recover the original bytes
// (e.g. `... | base64 -d > ecg.pdf`).
func Output(pdf []byte, outPath string) error {
	if outPath == "" {
		enc := base64.NewEncoder(base64.StdEncoding, os.Stdout)
		if _, err := enc.Write(pdf); err != nil {
			return err
		}
		if err := enc.Close(); err != nil {
			return err
		}
		_, err := fmt.Fprintln(os.Stdout)
		return err
	}
	return os.WriteFile(outPath, pdf, 0o644)
}

// --- header: patient identity (top-left) ---

func drawPatientBlock(pdf *fpdf.Fpdf, tr func(string) string, r *Report) {
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "", 8.5)
	const lh = 4.4

	// labelVal draws "label value" as plain selectable text at x, y.
	labelVal := func(x, y float64, label, value string) {
		pdf.SetXY(x, y)
		pdf.Write(lh, tr(label+" "+dash(value)))
	}
	// numUnit draws "value unit" (or "— unit" when empty) at x, y.
	numUnit := func(x, y float64, value, unit string) {
		pdf.SetXY(x, y)
		pdf.Write(lh, tr(dashUnit(value, unit)))
	}

	y := 7.0
	labelVal(margin, y, lbl.id, r.PatientID)
	y += lh
	labelVal(margin, y, lbl.name, r.Name)
	y += lh
	labelVal(margin, y, lbl.sex, r.Sex)
	labelVal(70, y, lbl.dob, fmtDOBraw(r.BirthDate))
	numUnit(125, y, r.Age, strings.TrimSpace(lbl.ageSuffix))
	y += lh
	numUnit(margin, y, r.Height, strings.TrimSpace(lbl.cmSuffix))
	numUnit(40, y, r.Weight, strings.TrimSpace(lbl.kgSuffix))
	numUnit(70, y, r.BloodPressure, "mmHg")
	y += lh
	labelVal(margin, y, lbl.meds, strings.Join(r.Medications, ", "))
	y += lh
	labelVal(margin, y, lbl.symptoms, r.Symptoms)
	y += lh
	labelVal(margin, y, lbl.history, r.History)
}

// --- header: interpretive statements (top-right) ---

func drawStatements(pdf *fpdf.Fpdf, tr func(string) string, r *Report) float64 {
	pdf.SetTextColor(0, 0, 0)
	const lh = 4.4
	y := 7.0
	for _, st := range r.Statements {
		if st.Text == "" {
			continue
		}
		if st.Emphasis {
			pdf.SetFont("Helvetica", "B", 8.5)
		} else {
			pdf.SetFont("Helvetica", "", 8.5)
		}
		if st.Code != "" {
			pdf.SetXY(rightX, y)
			pdf.Write(lh, tr(st.Code))
			pdf.SetXY(rightX+14, y)
		} else {
			pdf.SetXY(rightX, y)
		}
		pdf.Write(lh, tr(st.Text))
		y += lh
	}
	return y
}

// drawPhysicianBlock draws a wide "physician diagnosis" area to the right of the
// measurements, starting on the Ventricular-rate line: a bordered free-text zone
// plus a Physician/Date line, to be completed by hand on a printout.
func drawPhysicianBlock(pdf *fpdf.Fpdf, tr func(string) string) {
	const (
		lh        = 4.4
		x         = 98.0 // ~one tab right of the measurements' unit column
		yLabel    = 40.0 // same line as "Ventricular rate"
		boxTop    = 45.0
		boxBottom = 63.0
		sigY      = 64.5
	)
	wBlock := 297.0 - margin - x // run to the right page margin

	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "B", 8.5)
	pdf.SetXY(x, yLabel)
	pdf.Write(lh, tr(lbl.diag))

	pdf.SetDrawColor(150, 150, 150)
	pdf.SetLineWidth(0.2)
	pdf.Rect(x, boxTop, wBlock, boxBottom-boxTop, "D")

	// Signature line: Physician: ____   Date: ____
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetXY(x, sigY)
	pdf.Write(lh, tr(lbl.physician+" "))
	dateLabelX := x + wBlock - 45
	pdf.SetXY(dateLabelX, sigY)
	pdf.Write(lh, tr(lbl.dateL+" "))
}

// --- header: measurements table (label / right-aligned value / unit) ---

func drawMeasurements(pdf *fpdf.Fpdf, tr func(string) string, r *Report) {
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

	row(lbl.hr, fmt.Sprintf("%d", r.HeartRate), "bpm")
	row(lbl.prInt, fmt.Sprintf("%d", r.PRInterval), "ms")
	row(lbl.qrsDur, fmt.Sprintf("%d", r.QRSDuration), "ms")
	row(lbl.qtQtc, fmt.Sprintf("%d / %d", r.QTInterval, r.QTcInterval), "ms")
	row(lbl.axis, fmt.Sprintf("%d / %d / %d", r.PAxis, r.QRSAxis, r.TAxis), "°")
	if r.ShowAmplitudes {
		row(lbl.amplDiv, fmt.Sprintf("%.3f / %.3f", r.V5RAmplitude, r.V1SAmplitude), "mV")
		row(lbl.amplSum, fmt.Sprintf("%.3f", r.V5RAmplitude+r.V1SAmplitude), "mV")
	}
}

// --- header: scale / filter note above the signals ---

func drawScaleNote(pdf *fpdf.Fpdf, tr func(string) string, r *Report) {
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(0, 0, 0)
	note := fmt.Sprintf("%g mm/mV   %g mm/s   ", mmPerMV, mmPerSec)
	if r.Filter != "" {
		note += lbl.filter + " " + r.Filter
	}
	pdf.SetXY(40, gridTop-4)
	pdf.Write(4, tr(note))
	pdf.SetXY(gridX+gridW-18, gridTop-4)
	pdf.Write(4, tr(fmt.Sprintf("%g mm/mV", mmPerMV)))
}

// --- signals ---

func drawSignals(pdf *fpdf.Fpdf, r *Report, sr, scaleUV float64) {
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

	for r0 := 0; r0 < 3; r0++ {
		cellTop := gridTop + float64(r0)*rowH
		for c := 0; c < 4; c++ {
			name := layout[r0][c]
			x0 := gridX + float64(c)*colW
			startSec := float64(c) * secPerCol
			plotLead(pdf, r.Leads[name], sr, scaleUV, x0, cellTop, colW, rowH, startSec, secPerCol)
			leadLabel(pdf, name, x0+1, cellTop+1)
		}
	}

	// Rhythm strip: lead II across the full 10s.
	rhTop := gridTop + 3*rowH
	plotLead(pdf, r.Leads["II"], sr, scaleUV, gridX, rhTop, gridW, rhythmH, 0, gridW/mmPerSec)
	leadLabel(pdf, "II", gridX+1, rhTop+1)
}

// plotLead draws one lead trace as a single full-resolution vector path,
// clipped to its cell rectangle. Clipping hides any overflow cleanly without
// distorting tall QRS complexes into flat plateaus, so the trace stays faithful
// when zoomed.
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

// drawGrid paints the red ECG graph paper. Spacing is derived from the print
// scale so that one minor square is 0.04s / 0.1mV and one major square (every
// 5th) is 0.2s / 0.5mV, regardless of the chosen mm/s and mm/mV.
func drawGrid(pdf *fpdf.Fpdf, x, y, w, h float64) {
	minorX := 0.04 * mmPerSec // mm per 0.04s
	minorY := 0.10 * mmPerMV  // mm per 0.1mV

	pdf.SetLineWidth(0.1)
	pdf.SetDrawColor(255, 200, 200)
	for gx := 0.0; gx <= w+0.01; gx += minorX {
		pdf.Line(x+gx, y, x+gx, y+h)
	}
	for gy := 0.0; gy <= h+0.01; gy += minorY {
		pdf.Line(x, y+gy, x+w, y+gy)
	}
	pdf.SetLineWidth(0.3)
	pdf.SetDrawColor(255, 130, 130)
	for gx := 0.0; gx <= w+0.01; gx += 5 * minorX {
		pdf.Line(x+gx, y, x+gx, y+h)
	}
	for gy := 0.0; gy <= h+0.01; gy += 5 * minorY {
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

func drawFooter(pdf *fpdf.Fpdf, tr func(string) string, r *Report) {
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(0, 0, 0)
	y := gridTop + 3*rowH + rhythmH + 4

	left := fmt.Sprintf("%s   %s %s", dash(r.DeviceModel), lbl.service, dash(r.Department))
	pdf.SetXY(margin, y)
	pdf.Write(4, tr(left))

	right := fmt.Sprintf("%s %s, %s", lbl.exam, dash(r.Operator), dash(r.Location))
	pdf.SetXY(rightX, y)
	pdf.Write(4, tr(right))
}

// drawDisclaimer prints a centered notice at the very bottom of the page making
// clear this PDF is an automated, non-certified conversion and must not be used
// for diagnosis.
func drawDisclaimer(pdf *fpdf.Fpdf, tr func(string) string) {
	const pageHmm = 210.0 // A4 landscape height
	pdf.SetFont("Helvetica", "I", 6.5)
	pdf.SetTextColor(110, 110, 110)
	pdf.SetXY(margin, pageHmm-5.5)
	pdf.CellFormat(297.0-2*margin, 3, tr(lbl.disclaimer), "", 0, "C", false, 0, "")
}

// --- helpers ---

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// dashUnit renders "value unit", or "— unit" when the value is empty.
func dashUnit(v, unit string) string {
	if strings.TrimSpace(v) == "" {
		return "— " + unit
	}
	return v + " " + unit
}

// fmtDOBraw renders a YYYYMMDD birth date as YYYY-MM-DD, or "" when unknown
// (so an editable field stays blank rather than showing a dash).
func fmtDOBraw(b string) string {
	if len(b) != 8 {
		return ""
	}
	return b[:4] + "-" + b[4:6] + "-" + b[6:8]
}

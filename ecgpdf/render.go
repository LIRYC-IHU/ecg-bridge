package ecgpdf

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpumodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

const ptPerMM = 72.0 / 25.4 // PDF points per millimetre

// formField is one editable AcroForm text field, in fpdf coordinates
// (millimetres, top-left origin). Collected during drawing, overlaid afterwards.
type formField struct {
	id, value, align string
	x, y, w, h       float64
	multiline        bool
}

// Render-scoped form state (single-shot, single-threaded like lbl).
var (
	formsEnabled bool
	formFields   []formField
)

func regField(id string, x, y, w, h float64, value, align string) {
	if w < 4 {
		w = 4
	}
	if align == "" {
		align = "left"
	}
	formFields = append(formFields, formField{id: id, value: value, align: align, x: x, y: y, w: w, h: h})
}

// regFieldML registers a multiline editable field (e.g. a free-text area).
func regFieldML(id string, x, y, w, h float64) {
	formFields = append(formFields, formField{id: id, align: "left", x: x, y: y, w: w, h: h, multiline: true})
}

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
	formsEnabled = r.Forms
	formFields = nil
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
	yStmt := drawStatements(pdf, tr, r)
	drawPhysicianBlock(pdf, tr, yStmt+3)
	drawMeasurements(pdf, tr, r)
	drawScaleNote(pdf, tr, r)

	drawGrid(pdf, gridX, gridTop, gridW, 3*rowH+rhythmH)
	drawSignals(pdf, r, sr, scaleUV)

	drawFooter(pdf, tr, r)

	if !formsEnabled || len(formFields) == 0 {
		return pdf.Output(w)
	}
	var raw bytes.Buffer
	if err := pdf.Output(&raw); err != nil {
		return err
	}
	return overlayForms(raw.Bytes(), w)
}

// overlayForms stamps the collected editable fields onto the rendered PDF using
// pdfcpu's JSON "create" API (which merges form fields onto an existing page).
// fpdf uses a top-left mm origin; pdfcpu uses a bottom-left point origin.
func overlayForms(src []byte, w io.Writer) error {
	const pageHmm = 210.0 // A4 landscape height
	type tf struct {
		ID        string     `json:"id"`
		Value     string     `json:"value"`
		Pos       [2]float64 `json:"pos"`
		Width     float64    `json:"width"`
		Height    float64    `json:"height"`
		Align     string     `json:"align"`
		Multiline bool       `json:"multiline"`
	}
	tfs := make([]tf, 0, len(formFields))
	for _, f := range formFields {
		tfs = append(tfs, tf{
			ID:        f.id,
			Value:     f.value,
			Pos:       [2]float64{f.x * ptPerMM, (pageHmm - (f.y + f.h)) * ptPerMM},
			Width:     f.w * ptPerMM,
			Height:    f.h * ptPerMM,
			Align:     f.align,
			Multiline: f.multiline,
		})
	}
	doc := map[string]any{
		"origin": "LowerLeft",
		"fonts":  map[string]any{"input": map[string]any{"name": "Helvetica", "size": 9}},
		"pages":  map[string]any{"1": map[string]any{"content": map[string]any{"textfield": tfs}}},
	}
	js, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return api.Create(bytes.NewReader(src), bytes.NewReader(js), w, pdfcpumodel.NewDefaultConfiguration())
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

	// labelVal draws "label " then the value — as an editable field (forms on)
	// or plain text (forms off) — spanning from after the label to xEnd.
	labelVal := func(id string, x, y float64, label, value string, xEnd float64) {
		pdf.SetXY(x, y)
		if formsEnabled {
			pdf.Write(lh, tr(label+" "))
			fx := x + pdf.GetStringWidth(tr(label+" "))
			regField(id, fx, y, xEnd-fx, lh, value, "left")
		} else {
			pdf.Write(lh, tr(label+" "+dash(value)))
		}
	}
	// numUnit draws a small value (field or text) at x, then a static unit.
	numUnit := func(id string, x, y, w float64, value, unit string) {
		if formsEnabled {
			regField(id, x, y, w, lh, value, "left")
			pdf.SetXY(x+w+1, y)
			pdf.Write(lh, tr(unit))
		} else {
			pdf.SetXY(x, y)
			pdf.Write(lh, tr(dashUnit(value, unit)))
		}
	}

	y := 7.0
	labelVal("pid", margin, y, lbl.id, r.PatientID, 120)
	y += lh
	labelVal("name", margin, y, lbl.name, r.Name, 120)
	y += lh
	labelVal("sex", margin, y, lbl.sex, r.Sex, 60)
	labelVal("dob", 70, y, lbl.dob, fmtDOBraw(r.BirthDate), 118)
	numUnit("age", 125, y, 12, r.Age, strings.TrimSpace(lbl.ageSuffix))
	y += lh
	numUnit("height", margin, y, 16, r.Height, strings.TrimSpace(lbl.cmSuffix))
	numUnit("weight", 40, y, 16, r.Weight, strings.TrimSpace(lbl.kgSuffix))
	numUnit("bp", 70, y, 16, "", "mmHg")
	y += lh
	labelVal("meds", margin, y, lbl.meds, strings.Join(r.Medications, ", "), 195)
	y += lh
	labelVal("symptoms", margin, y, lbl.symptoms, r.Symptoms, 195)
	y += lh
	labelVal("history", margin, y, lbl.history, r.History, 195)
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

// drawPhysicianBlock draws a "physician diagnosis" area below the interpretive
// statements (right column): a bordered free-text zone — an editable multiline
// field when forms are on — plus a Physician/Date signature line.
func drawPhysicianBlock(pdf *fpdf.Fpdf, tr func(string) string, yStart float64) {
	const (
		lh         = 4.4
		areaBottom = 57.0 // keep clear of the signals/scale note below
		sigY       = 60.0
	)
	x := rightX
	wBlock := 297.0 - margin - rightX // right column width

	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "B", 8.5)
	pdf.SetXY(x, yStart)
	pdf.Write(lh, tr(lbl.diag))

	areaTop := yStart + lh + 0.5
	if areaTop > areaBottom-10 {
		areaTop = areaBottom - 10 // guarantee a minimum writing height
	}
	pdf.SetDrawColor(150, 150, 150)
	pdf.SetLineWidth(0.2)
	pdf.Rect(x, areaTop, wBlock, areaBottom-areaTop, "D")
	if formsEnabled {
		regFieldML("diag", x+0.5, areaTop+0.5, wBlock-1, areaBottom-areaTop-1)
	}

	// Signature line: Physician: [____]   Date: [____]
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetXY(x, sigY)
	pdf.Write(lh, tr(lbl.physician+" "))
	dateLabelX := x + wBlock - 30
	if formsEnabled {
		pw := pdf.GetStringWidth(tr(lbl.physician + " "))
		regField("md_physician", x+pw, sigY, dateLabelX-(x+pw)-2, lh, "", "left")
	}
	pdf.SetXY(dateLabelX, sigY)
	pdf.Write(lh, tr(lbl.dateL+" "))
	if formsEnabled {
		dw := pdf.GetStringWidth(tr(lbl.dateL + " "))
		regField("md_date", dateLabelX+dw, sigY, (x+wBlock)-(dateLabelX+dw), lh, "", "left")
	}
}

// --- header: measurements table (label / right-aligned value / unit) ---

func drawMeasurements(pdf *fpdf.Fpdf, tr func(string) string, r *Report) {
	pdf.SetFont("Helvetica", "", 8.5)
	pdf.SetTextColor(0, 0, 0)

	y := 40.0
	const lh = 4.3
	row := func(id, label, value, unit string) {
		pdf.SetXY(margin, y)
		pdf.CellFormat(46, lh, tr(label), "", 0, "L", false, 0, "")
		if formsEnabled {
			regField(id, 54, y, 24, lh, value, "right")
		} else {
			pdf.SetXY(54, y)
			pdf.CellFormat(24, lh, tr(value), "", 0, "R", false, 0, "")
		}
		pdf.SetXY(80, y)
		pdf.CellFormat(16, lh, tr(unit), "", 0, "L", false, 0, "")
		y += lh
	}

	row("hr", lbl.hr, fmt.Sprintf("%d", r.HeartRate), "bpm")
	row("pr", lbl.prInt, fmt.Sprintf("%d", r.PRInterval), "ms")
	row("qrs", lbl.qrsDur, fmt.Sprintf("%d", r.QRSDuration), "ms")
	row("qtqtc", lbl.qtQtc, fmt.Sprintf("%d / %d", r.QTInterval, r.QTcInterval), "ms")
	row("axis", lbl.axis, fmt.Sprintf("%d / %d / %d", r.PAxis, r.QRSAxis, r.TAxis), "°")
	if r.ShowAmplitudes {
		row("ampldiv", lbl.amplDiv, fmt.Sprintf("%.3f / %.3f", r.V5RAmplitude, r.V1SAmplitude), "mV")
		row("amplsum", lbl.amplSum, fmt.Sprintf("%.3f", r.V5RAmplitude+r.V1SAmplitude), "mV")
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

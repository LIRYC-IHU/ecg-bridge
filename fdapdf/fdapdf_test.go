package fdapdf

import (
	"bytes"
	"compress/zlib"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/LIRYC-IHU/ecg-bridge/ecgpdf"
	fdatodicom "github.com/LIRYC-IHU/ecg-bridge/fda-to-dicom"
)

// This package is the join between the aECG parser and the renderer, and it had
// no tests. The consequence was a chain whose two ends were covered and whose
// middle was not: severing the per-lead scale here left the whole suite green.
//
// The fixtures are synthetic aECG documents — a generated triangle ramp, no
// patient data — so everything here runs in CI.

const (
	uniformFixture = "../fda-to-dicom/testdata/aecg_uniform_scale.xml"
	perLeadFixture = "../fda-to-dicom/testdata/aecg_per_lead_scale.xml"
)

// FromFDA must carry each lead's own amplitude scale into the report. Without
// it the renderer applies the document's reference scale to every lead, which
// rescales any lead digitised differently.
func TestFromFDACarriesPerLeadScales(t *testing.T) {
	d := &fdatodicom.FDAData{
		SamplingRate:    500,
		Sensitivity:     5,
		LeadSensitivity: map[string]float64{"I": 5, "V1": 2.5},
		Leads:           map[string][]int32{"I": {1, 2, 3}, "V1": {2, 4, 6}},
	}

	rep := FromFDA(d)

	if rep.ScaleUV != 5 {
		t.Errorf("reference scale = %v, want 5", rep.ScaleUV)
	}
	if rep.ScaleUVByLead == nil {
		t.Fatal("per-lead scales were dropped between FDAData and Report")
	}
	if got := rep.ScaleUVByLead["V1"]; got != 2.5 {
		t.Errorf("V1 scale = %v, want 2.5", got)
	}
	if got := rep.ScaleUVByLead["I"]; got != 5 {
		t.Errorf("I scale = %v, want 5", got)
	}
}

// A document stating one scale for every lead must not gain a spurious map —
// leads absent from it fall back to the reference scale, which is the common case.
func TestFromFDALeavesPerLeadScalesNilWhenAbsent(t *testing.T) {
	d := &fdatodicom.FDAData{
		SamplingRate: 500,
		Sensitivity:  5,
		Leads:        map[string][]int32{"I": {1, 2, 3}},
	}
	if rep := FromFDA(d); rep.ScaleUVByLead != nil {
		t.Errorf("ScaleUVByLead = %v, want nil when the document states no per-lead scale", rep.ScaleUVByLead)
	}
}

// --- the whole chain, from XML to rendered page ----------------------------

// The end-to-end property, over every join at once: parser → FDAData →
// Report → renderer.
//
// Two aECG documents describe the same recording. In one, V1 is digitised at
// 5 µV/LSB. In the other, at 2.5 µV/LSB carrying twice the digits — the same
// voltage, expressed differently. The rendered pages must be identical.
//
// This is the test that fails if any link in the chain drops the per-lead
// scale, which the unit tests at either end do not catch.
func TestPerLeadScaleSurvivesTheWholeChain(t *testing.T) {
	uniform := renderContent(t, uniformFixture)
	perLead := renderContent(t, perLeadFixture)

	if !bytes.Equal(uniform, perLead) {
		t.Error("the same recording digitised at different per-lead scales renders differently; " +
			"a link in parser → FDAData → Report → renderer is dropping the per-lead scale")
	}
}

// Guard: the two fixtures must not be trivially identical, or the test above
// proves nothing. They differ in V1's scale and digits, so a renderer that
// ignored the scale would produce V1 at twice the amplitude.
func TestFixturesDifferInTheirEncoding(t *testing.T) {
	a := readFixture(t, uniformFixture)
	b := readFixture(t, perLeadFixture)
	if bytes.Equal(a, b) {
		t.Fatal("the two fixtures are byte-identical; the chain test compares nothing")
	}

	du, err := fdatodicom.ParseFDA(filepath.Clean(uniformFixture))
	if err != nil {
		t.Fatalf("parsing uniform fixture: %v", err)
	}
	dp, err := fdatodicom.ParseFDA(filepath.Clean(perLeadFixture))
	if err != nil {
		t.Fatalf("parsing per-lead fixture: %v", err)
	}
	if du.LeadSensitivity["V1"] == dp.LeadSensitivity["V1"] {
		t.Fatal("both fixtures declare the same V1 scale; the chain test compares nothing")
	}
	if len(dp.Leads["V1"]) == 0 {
		t.Fatal("the per-lead fixture decoded no V1 samples; the render would compare two blank columns")
	}
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return b
}

// renderContent renders a fixture and returns the page's drawing commands.
//
// It compares the content stream rather than the PDF bytes, because the file as
// a whole is NOT reproducible: rendering the same input twice yields two files
// of identical length and differing bytes, as fpdf writes its font resource
// dictionary in map order. The first version of this test compared raw bytes
// and was flaky — it passed locally and failed on CI, which is the worst way to
// find out. The drawing commands, which are what this test is actually about,
// are deterministic.
func renderContent(t *testing.T, path string) []byte {
	t.Helper()
	rep, err := ReportFromFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("ReportFromFile(%s): %v", path, err)
	}
	var buf bytes.Buffer
	if err := ecgpdf.Render(rep, "en", &buf); err != nil {
		t.Fatalf("Render(%s): %v", path, err)
	}

	// The page content stream is by far the largest deflated stream in the
	// file; the others are font data.
	var content []byte
	rest := buf.Bytes()
	for {
		i := bytes.Index(rest, []byte("stream"))
		if i < 0 {
			break
		}
		body := bytes.TrimLeft(rest[i+len("stream"):], "\r\n")
		j := bytes.Index(body, []byte("endstream"))
		if j < 0 {
			break
		}
		if zr, err := zlib.NewReader(bytes.NewReader(body[:j])); err == nil {
			if dec, err := io.ReadAll(zr); err == nil && len(dec) > len(content) {
				content = dec
			}
			zr.Close()
		}
		rest = body[j:]
	}
	if len(content) == 0 {
		t.Fatalf("no content stream could be inflated from the render of %s", path)
	}
	return content
}

// The comparison above is only meaningful if rendering is reproducible at all.
// Pin that separately, so a future non-determinism in the drawing commands
// shows up as itself rather than as a confusing per-lead failure.
func TestRenderingTheSameFixtureTwiceIsReproducible(t *testing.T) {
	if !bytes.Equal(renderContent(t, uniformFixture), renderContent(t, uniformFixture)) {
		t.Error("rendering the same document twice produced different drawing commands")
	}
}

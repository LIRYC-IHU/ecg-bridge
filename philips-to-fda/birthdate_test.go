package philipstofda

import (
	"strings"
	"testing"

	"github.com/LIRYC-IHU/ecg-bridge/metaject"
	philipstodicom "github.com/LIRYC-IHU/ecg-bridge/philips-to-dicom"
)

// A Philips SierraECG file carries no date of birth, so this converter hardcoded
// an empty one and then dropped the birthTime element outright. An injected date
// could go in and never came out: the rendered report showed the HIS name above
// the acquisition device's age — the mismatch a clinician cross-checks an
// identity on.
//
// buildAECG is exercised directly rather than through Convert: the assertion is
// about what reaches the document, and the repository carries no Philips sample
// to parse.

func TestBuildAECG_EmitsAnInjectedBirthDate(t *testing.T) {
	doc := build(t, &philipstodicom.PhilipsData{
		PatientID:   "BS1170",
		PatientName: "Fontaine^Sébastien",
		PatientSex:  "M",
		PatientDOB:  "19680422",
		StudyDate:   "20260917",
		StudyTime:   "120000",
		StudyUID:    "1.2.3.4",
	})

	if !strings.Contains(doc, "birthTime") {
		t.Errorf("no birthTime element was emitted:\n%s", demographics(doc))
	}
	if !strings.Contains(doc, "19680422") {
		t.Errorf("the date of birth is absent from the document:\n%s", demographics(doc))
	}
}

func TestBuildAECG_OmitsBirthTimeWhenThereIsNoDate(t *testing.T) {
	// The element must stay absent rather than appear empty, which a reader
	// could take for a known value.
	doc := build(t, &philipstodicom.PhilipsData{
		PatientID: "BS1170", PatientName: "BLIN^Jean Michel",
		StudyDate: "20260917", StudyTime: "120000", StudyUID: "1.2.3.4",
	})

	if strings.Contains(doc, "birthTime") {
		t.Errorf("birthTime emitted with nothing to put in it:\n%s", demographics(doc))
	}
}

func TestApplyMetadata_TakesTheBirthDate(t *testing.T) {
	d := &philipstodicom.PhilipsData{PatientDOB: "19000101"}
	dob := "19680422"
	d.ApplyMetadata(&metaject.Override{BirthDate: &dob})

	if d.PatientDOB != dob {
		t.Errorf("PatientDOB = %q, want %q", d.PatientDOB, dob)
	}
}

func TestAnonymize_ClearsTheBirthDate(t *testing.T) {
	// Anonymisation on its own must not leave an identifying date behind.
	d := &philipstodicom.PhilipsData{PatientName: "X", PatientID: "Y", PatientDOB: "19680422"}
	d.Anonymize()

	if d.PatientDOB != "" {
		t.Errorf("PatientDOB = %q after Anonymize, want empty", d.PatientDOB)
	}
}

func build(t *testing.T, d *philipstodicom.PhilipsData) string {
	t.Helper()
	doc, err := buildAECG(d)
	if err != nil {
		t.Fatalf("buildAECG: %v", err)
	}
	return doc
}

// demographics returns the identity block, so a failure shows it rather than the
// whole document.
func demographics(doc string) string {
	i := strings.Index(doc, "subjectDemographicPerson")
	if i < 0 {
		i = 0
	}
	end := i + 500
	if end > len(doc) {
		end = len(doc)
	}
	return doc[i:end]
}

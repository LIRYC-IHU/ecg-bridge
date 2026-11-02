package dicomconf

import (
	"testing"

	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/tag"
)

func TestFinalizeCharacterSet(t *testing.T) {
	name, _ := dicom.NewElement(tag.PatientName, []string{"H\xe9l\xe8ne^Andr\xe9"}) // Latin-1 bytes
	utf, _ := dicom.NewElement(tag.InstitutionName, []string{"Hôpital Haut-Lévêque"})
	ds := &dicom.Dataset{Elements: []*dicom.Element{name, utf}}

	Finalize(ds)

	want := map[tag.Tag]string{
		tag.SpecificCharacterSet: "ISO_IR 192",
		tag.PatientName:          "Hélène^André",
		tag.InstitutionName:      "Hôpital Haut-Lévêque",
	}
	for tg, w := range want {
		el, err := ds.FindElementByTag(tg)
		if err != nil {
			t.Fatalf("%v missing", tg)
		}
		if got := el.Value.GetValue().([]string)[0]; got != w {
			t.Errorf("%v = %q, want %q", tg, got, w)
		}
	}
}

func TestFinalizeType2(t *testing.T) {
	dob, _ := dicom.NewElement(tag.PatientBirthDate, []string{"19600101"})
	ds := &dicom.Dataset{Elements: []*dicom.Element{dob}}

	Finalize(ds)

	for _, tg := range []tag.Tag{
		tag.ReferringPhysicianName, tag.AccessionNumber, tag.StudyID, tag.AcquisitionContextSequence,
	} {
		if _, err := ds.FindElementByTag(tg); err != nil {
			t.Errorf("Type 2 %v missing", tg)
		}
	}
	el, _ := ds.FindElementByTag(tag.PatientBirthDate)
	if got := el.Value.GetValue().([]string)[0]; got != "19600101" {
		t.Errorf("existing birth date overwritten: %q", got)
	}
}

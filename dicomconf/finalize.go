// Package dicomconf applies shared conformance fix-ups to DICOM datasets built
// by the ECG converters, so every output carries the required File Meta
// elements, keeps elements in ascending tag order, declares a character set,
// and uses the correct value representation for waveform data.
//
// All converters build their dataset and then call Finalize just before
// writing, which keeps these cross-cutting DICOM rules in one place.
package dicomconf

import (
	"sort"

	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/tag"
)

const (
	// implementationClassUID identifies this converter suite as the creating
	// implementation. File Meta (0002,0012) is Type 1 (required); it must be
	// stable across runs, so it is a fixed UID rather than generated.
	implementationClassUID = "2.25.83057759282917960894454627989"

	// implementationVersionName is File Meta (0002,0013), Type 3.
	implementationVersionName = "ECGHUB-CONV-1"

	// defaultCharacterSet declares Latin-1 so accented patient names are
	// interpreted correctly by receivers (0008,0005).
	defaultCharacterSet = "ISO_IR 100"

	vrOtherWord = "OW"
)

// Finalize applies the conformance fix-ups in place. Call it on the fully built
// dataset immediately before dicom.Write.
func Finalize(ds *dicom.Dataset) {
	if ds == nil {
		return
	}

	ensureString(ds, tag.SpecificCharacterSet, defaultCharacterSet)
	ensureString(ds, tag.ImplementationClassUID, implementationClassUID)
	ensureString(ds, tag.ImplementationVersionName, implementationVersionName)

	// Waveform sample data is 16-bit, so its VR must be OW. Builders create it
	// from a byte slice (which defaults to OB); correct it wherever it appears,
	// including inside the WaveformSequence items.
	fixupElements(ds.Elements)

	// A DICOM data set must list its elements in ascending tag order.
	sortElements(ds.Elements)
}

func hasTag(ds *dicom.Dataset, t tag.Tag) bool {
	for _, e := range ds.Elements {
		if e.Tag == t {
			return true
		}
	}
	return false
}

func ensureString(ds *dicom.Dataset, t tag.Tag, v string) {
	if hasTag(ds, t) {
		return
	}
	if el, err := dicom.NewElement(t, []string{v}); err == nil {
		ds.Elements = append(ds.Elements, el)
	}
}

// fixupElements recurses into sequence items, sorting each item's elements into
// ascending tag order and forcing the waveform-data VR to OW.
func fixupElements(elems []*dicom.Element) {
	for _, e := range elems {
		if e.Tag == tag.WaveformData {
			e.RawValueRepresentation = vrOtherWord
		}
		if e.Value == nil || e.Value.ValueType() != dicom.Sequences {
			continue
		}
		items, ok := e.Value.GetValue().([]*dicom.SequenceItemValue)
		if !ok {
			continue
		}
		for _, item := range items {
			sub, ok := item.GetValue().([]*dicom.Element)
			if !ok {
				continue
			}
			fixupElements(sub)
			sortElements(sub)
		}
	}
}

func sortElements(elems []*dicom.Element) {
	sort.SliceStable(elems, func(i, j int) bool {
		a, b := elems[i].Tag, elems[j].Tag
		if a.Group != b.Group {
			return a.Group < b.Group
		}
		return a.Element < b.Element
	})
}

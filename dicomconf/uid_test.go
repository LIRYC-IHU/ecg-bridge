package dicomconf

import (
	"regexp"
	"testing"
)

func TestDeriveUIDs(t *testing.T) {
	src := []byte("same ecg")
	a := DeriveUIDs(src, false)

	if a != DeriveUIDs(src, false) {
		t.Fatal("same source must yield the same UIDs")
	}
	if a.Study == a.Series || a.Study == a.SOP || a.Series == a.SOP {
		t.Fatalf("study, series and SOP UIDs must differ: %+v", a)
	}
	if b := DeriveUIDs([]byte("other ecg"), false); b.Study == a.Study || b.SOP == a.SOP {
		t.Fatal("different sources must yield different UIDs")
	}
	if c := DeriveUIDs(src, true); c.Study == a.Study || c.SOP == a.SOP {
		t.Fatal("anonymized copy must not reuse the identified UIDs")
	}

	// PS3.5 §9.1: digits and dots, no leading zero per component, ≤ 64 chars.
	valid := regexp.MustCompile(`^2\.25\.(0|[1-9][0-9]*)$`)
	for _, u := range []string{a.Study, a.Series, a.SOP} {
		if !valid.MatchString(u) || len(u) > 64 {
			t.Fatalf("invalid UID %q", u)
		}
	}
}

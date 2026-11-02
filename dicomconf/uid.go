package dicomconf

import (
	"crypto/sha1"
	"crypto/sha256"
	"math/big"
)

// UIDs holds the instance identifiers of one converted ECG.
type UIDs struct {
	Study, Series, SOP string
}

// uidNamespace is the UUID namespace (ISO/IEC 9834-8 name-based) under which
// every converter derives its UIDs. It must never change: doing so would give
// already-archived ECGs new identities on their next conversion.
var uidNamespace = [16]byte{
	0x5b, 0x0e, 0x3c, 0x6a, 0x8f, 0x21, 0x4d, 0x7e,
	0x9a, 0x13, 0x62, 0xc4, 0xe0, 0x57, 0xb1, 0x9d,
}

// DeriveUIDs derives Study, Series and SOP Instance UIDs from the source file
// bytes, so converting (or re-forwarding) the same ECG always yields the same
// identifiers instead of a duplicate study in the PACS. The anonymized flag is
// part of the key: an anonymized copy is a different instance and must not
// collide with the identified one.
//
// Each UID is "2.25.<UUID as integer>" (PS3.5 B.2) where the UUID is version 5
// (SHA-1, name-based) over the source digest and the UID's role.
func DeriveUIDs(source []byte, anonymized bool) UIDs {
	digest := sha256.Sum256(source)
	key := digest[:]
	if anonymized {
		key = append(key, 'A')
	}
	return UIDs{
		Study:  nameUID(key, "study"),
		Series: nameUID(key, "series"),
		SOP:    nameUID(key, "sop"),
	}
}

func nameUID(key []byte, role string) string {
	h := sha1.New()
	h.Write(uidNamespace[:])
	h.Write(key)
	h.Write([]byte(role))
	u := h.Sum(nil)[:16]
	u[6] = u[6]&0x0f | 0x50 // version 5
	u[8] = u[8]&0x3f | 0x80 // RFC 4122 variant
	return "2.25." + new(big.Int).SetBytes(u).String()
}

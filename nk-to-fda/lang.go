package nktofda

import (
	_ "embed"
	"strings"
	"unicode/utf16"
)

// Diagnostic statement code system OID (NK proprietary statement vocabulary).
const statementCodeSystem = "1.2.392.200070.77706982"

// Embedded statement vocabularies. Only English and French are shipped.
//
//go:embed langdata/EcgToMfrLang_en.txt
var langEN []byte

//go:embed langdata/EcgToMfrLang_fr.txt
var langFR []byte

// statementTables maps a language tag to its decoded code→text lookup.
// Built lazily on first use.
var statementTables = map[string]map[string]string{}

// supportedLangs lists the language tags we resolve statement text for.
var supportedLangs = map[string][]byte{
	"en": langEN,
	"fr": langFR,
}

// NormalizeLang returns a supported language tag ("en" or "fr"), defaulting to
// "en" for anything unrecognized.
func NormalizeLang(lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	if len(l) > 2 {
		l = l[:2]
	}
	if _, ok := supportedLangs[l]; ok {
		return l
	}
	return "en"
}

// statementText resolves a statement code to its human-readable text in the
// given language. Returns "" when the code is unknown.
func statementText(lang, code string) string {
	lang = NormalizeLang(lang)
	tbl, ok := statementTables[lang]
	if !ok {
		tbl = parseLangTable(supportedLangs[lang])
		statementTables[lang] = tbl
	}
	return tbl[code]
}

// parseLangTable decodes a UTF-16LE vocabulary file into a code→text map.
//
// File format: a leading entry-count line, then one entry per line as
// "<code>\t<text>" with CRLF terminators.
func parseLangTable(raw []byte) map[string]string {
	m := make(map[string]string)
	text := decodeUTF16LE(raw)
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		ln = strings.TrimRight(ln, "\r")
		if i == 0 {
			// first line is the entry count
			continue
		}
		tab := strings.IndexByte(ln, '\t')
		if tab < 0 {
			continue
		}
		code := strings.TrimSpace(ln[:tab])
		val := strings.TrimSpace(ln[tab+1:])
		if code != "" {
			m[code] = val
		}
	}
	return m
}

// decodeUTF16LE converts a UTF-16 little-endian byte slice (optional BOM) to a
// UTF-8 string.
func decodeUTF16LE(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		b = b[2:]
	}
	u16 := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u16 = append(u16, uint16(b[i])|uint16(b[i+1])<<8)
	}
	return string(utf16.Decode(u16))
}

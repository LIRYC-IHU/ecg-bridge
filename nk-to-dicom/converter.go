package nktodicom

import (
	"fmt"
	"os"

	nktofda "converter-fda/nk-to-fda"

	"github.com/suyashkumar/dicom"
)

// Convert reads a NK .DAT file and writes a DICOM ECG file.
func Convert(inputPath, outputPath string) error {
	// 1. Read and parse NK file
	dat, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", inputPath, err)
	}

	nd, err := nktofda.ParseFile(dat)
	if err != nil {
		return fmt.Errorf("parsing NK file: %w", err)
	}

	// 2. Decode waveforms
	secs, err := parseSections(dat)
	if err != nil {
		return err
	}
	recSec := secs[0x0008] // RECORD section
	recData := dat[recSec.offset+14:]
	leads, err := nktofda.DecodeLeads(recData, nd.Record.TotalSamples)
	if err != nil {
		return fmt.Errorf("decoding waveforms: %w", err)
	}
	nd.Leads = leads

	// 3. Build DICOM dataset
	ds, err := BuildDICOM(nd)
	if err != nil {
		return fmt.Errorf("building DICOM: %w", err)
	}

	// 4. Write DICOM file
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer f.Close()

	if err := dicom.Write(f, *ds); err != nil {
		return fmt.Errorf("writing DICOM: %w", err)
	}

	return nil
}

// parseSections is a helper to parse PEC sections (copied from nk-to-fda for now)
func parseSections(dat []byte) (map[uint16]section, error) {
	sections := make(map[uint16]section)
	off := 0
	for off+14 < len(dat) {
		if off+14 > len(dat) {
			break
		}
		size := int(dat[off+10])<<8 | int(dat[off+11])
		stype := uint16(dat[off+12])<<8 | uint16(dat[off+13])
		if size < 14 {
			break
		}
		dataEnd := off + size
		if dataEnd > len(dat) {
			break
		}
		sections[stype] = section{
			offset: uint32(off),
			data:   dat[off+14:dataEnd],
		}
		off = dataEnd + 2
	}
	if _, ok := sections[0x0008]; !ok {
		return nil, fmt.Errorf("RECORD section not found")
	}
	return sections, nil
}

type section struct {
	offset uint32
	data   []byte
}

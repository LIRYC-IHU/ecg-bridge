package nktodicom

import (
	nktofda "github.com/LIRYC-IHU/ecg-bridge/nk-to-fda"
)

// NKDICOMData wraps NKData with DICOM-specific metadata.
type NKDICOMData struct {
	NK *nktofda.NKData

	// DICOM UIDs (generated at conversion time)
	StudyInstanceUID  string
	SeriesInstanceUID string
	SOPInstanceUID    string

	// Derived from NK data
	StudyDate string // YYYYMMDD
	StudyTime string // HHMMSS
}

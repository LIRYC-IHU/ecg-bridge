package dicomtofda

import (
	"fmt"
	"io"
)

// PrintDebug writes a human-readable summary of parsed DICOM data to w.
func PrintDebug(data *DicomData, w io.Writer) {
	fmt.Fprintln(w, "=== DICOM DEBUG ===")

	fmt.Fprintln(w, "\n[Patient]")
	fmt.Fprintf(w, "  Name:        %s\n", orEmpty(data.Patient.PatientName))
	fmt.Fprintf(w, "  ID:          %s\n", orEmpty(data.Patient.PatientID))
	fmt.Fprintf(w, "  BirthDate:   %s\n", orEmpty(data.Patient.PatientBirthDate))
	fmt.Fprintf(w, "  Sex:         %s\n", orEmpty(data.Patient.PatientSex))
	fmt.Fprintf(w, "  Age:         %s\n", orEmpty(data.Patient.PatientAge))
	fmt.Fprintf(w, "  Institution: %s\n", orEmpty(data.Patient.InstitutionName))
	fmt.Fprintf(w, "  Device:      %s\n", orEmpty(data.Patient.DeviceModel))
	fmt.Fprintf(w, "  Serial:      %s\n", orEmpty(data.Patient.DeviceSerial))
	fmt.Fprintf(w, "  Software:    %s\n", orEmpty(data.Patient.SoftwareVersion))
	fmt.Fprintf(w, "  Manufacturer:%s\n", orEmpty(data.Patient.Manufacturer))
	fmt.Fprintf(w, "  Operator:    %s\n", orEmpty(data.Patient.OperatorsName))
	fmt.Fprintf(w, "  StudyDate:   %s\n", orEmpty(data.StudyDate))
	fmt.Fprintf(w, "  StudyTime:   %s\n", orEmpty(data.StudyTime))
	fmt.Fprintf(w, "  StudyUID:    %s\n", orEmpty(data.StudyInstanceUID))

	fmt.Fprintf(w, "\n[Waveforms] %d group(s)\n", len(data.Waveforms))
	for i, wf := range data.Waveforms {
		fmt.Fprintf(w, "  Group %d:\n", i)
		fmt.Fprintf(w, "    SamplingFrequency:    %.2f Hz\n", wf.SamplingFrequency)
		fmt.Fprintf(w, "    NumberOfChannels:     %d\n", wf.NumberOfChannels)
		fmt.Fprintf(w, "    NumberOfSamples:      %d\n", wf.NumberOfSamples)
		fmt.Fprintf(w, "    BitsAllocated:        %d\n", wf.BitsAllocated)
		fmt.Fprintf(w, "    SampleInterpretation: %s\n", orEmpty(wf.SampleInterpretation))
		fmt.Fprintf(w, "    RawDataBytes:         %d\n", len(wf.RawData))
		if len(wf.Channels) > 0 {
			fmt.Fprintf(w, "    Channels (%d):\n", len(wf.Channels))
			for j, ch := range wf.Channels {
				fmt.Fprintf(w, "      [%d] label=%-6s source=%-6s sensitivity=%g %s baseline=%g  LPF=%g HPF=%g notch=%g\n",
					j,
					orEmpty(ch.Label),
					orEmpty(ch.SourceName),
					ch.Sensitivity,
					orEmpty(ch.SensitivityUnit),
					ch.Baseline,
					ch.FilterLowFrequency,
					ch.FilterHighFrequency,
					ch.NotchFilterFrequency,
				)
			}
		} else {
			fmt.Fprintln(w, "    Channels: (no ChannelDefinitionSequence)")
		}
	}

	fmt.Fprintf(w, "\n[Annotations] %d item(s)\n", len(data.Annotations))
	for _, ann := range data.Annotations {
		if ann.NumericValue != "" {
			fmt.Fprintf(w, "  %-30s = %s %s\n", ann.ConceptName, ann.NumericValue, ann.Unit)
		} else if ann.TextValue != "" {
			fmt.Fprintf(w, "  %-30s : %s\n", ann.ConceptName, ann.TextValue)
		} else {
			fmt.Fprintf(w, "  %-30s (empty)\n", ann.ConceptName)
		}
	}

	fmt.Fprintln(w, "\n===================")
}

func orEmpty(s string) string {
	if s == "" {
		return "(empty)"
	}
	return s
}

package philipstodicom

import "encoding/xml"

// PhilipsXML is the root struct for Philips SierraECG XML (v1.03/1.04).
type PhilipsXML struct {
	XMLName         xml.Name        `xml:"http://www3.medical.philips.com restingecgdata"`
	DocumentInfo    DocumentInfo    `xml:"documentinfo"`
	Patient         Patient         `xml:"patient"`
	DataAcq         DataAcq         `xml:"dataacquisition"`
	ReportInfo      ReportInfo      `xml:"reportinfo"`
	Measurements    Measurements    `xml:"measurements"`
	Waveforms       Waveforms       `xml:"waveforms"`
	Interpretations Interpretations `xml:"interpretations"`
}

type DocumentInfo struct {
	Version string `xml:"documentversion"`
}

type Patient struct {
	General PatientGeneral `xml:"generalpatientdata"`
}

type PatientGeneral struct {
	PatientID  string      `xml:"patientid"`
	Name       PatientName `xml:"name"`
	Age        PatientAge  `xml:"age"`
	Sex        string      `xml:"sex"`
	PaceStatus string      `xml:"pacestatus"`
}

type PatientName struct {
	Last  string `xml:"lastname"`
	First string `xml:"firstname"`
}

type PatientAge struct {
	Default string `xml:"defaultage,attr"`
}

type DataAcq struct {
	Date     string     `xml:"date,attr"`
	Time     string     `xml:"time,attr"`
	Machine  Machine    `xml:"machine"`
	Acquirer Acquirer   `xml:"acquirer"`
	Signal   SignalChar `xml:"signalcharacteristics"`
}

type Machine struct {
	Detail string `xml:"detaildescription,attr"`
	Name   string `xml:",chardata"`
}

type Acquirer struct {
	InstitutionName string `xml:"institutionname"`
	OperatorID      string `xml:"operatorid"`
	Room            string `xml:"room"`
}

type SignalChar struct {
	SamplingRate  string `xml:"samplingrate"`
	Resolution    string `xml:"signalresolution"`
	HiPass        string `xml:"hipass"`
	LowPass       string `xml:"lowpass"`
	Bandwidth     string `xml:"signalbandwidth"`
	AcSetting     string `xml:"acsetting"`
	BitsPerSample string `xml:"bitspersample"`
	Channels      string `xml:"numberchannelsvalid"`
	SignalOffset  string `xml:"signaloffset"`
}

type ReportInfo struct {
	Bandwidth ReportBandwidth `xml:"reportbandwidth"`
}

type ReportBandwidth struct {
	HPF   string `xml:"highpassfiltersetting"`
	LPF   string `xml:"lowpassfiltersetting"`
	Notch string `xml:"notchfiltersetting"`
}

type Measurements struct {
	Global        GlobalMeasurements `xml:"globalmeasurements"`
	GroupMeasures GroupMeasurements  `xml:"groupmeasurements"`
}

// GroupMeasurements wraps the groupmeasurements container.
type GroupMeasurements struct {
	Items []GroupMeasurement `xml:"groupmeasurement"`
}

type GlobalMeasurements struct {
	MeanPR       string `xml:"meanprint"`
	MeanQRS      string `xml:"meanqrsdur"`
	MeanQT       string `xml:"meanqtint"`
	MeanQTc      string `xml:"meanqtc"`
	PFrontAxis   string `xml:"pfrontaxis"`
	QRSFrontAxis string `xml:"qrsfrontaxis"`
	TFrontAxis   string `xml:"tfrontaxis"`
	STFrontAxis  string `xml:"stfrontaxis"`
	AtrialRate   string `xml:"atrialrate"`
	QTDispersion string `xml:"qtintdispersion"`
}

// GroupMeasurement holds per-group measurements.
type GroupMeasurement struct {
	MeanRRInt string `xml:"meanrrint"`
}

type Interpretations struct {
	Interpretation InterpretationItem `xml:"interpretation"`
}

type InterpretationItem struct {
	Measurements    InterpretationMeasurements `xml:"interpretationmeasurements"`
	MdSignatureLine string                     `xml:"mdsignatureline"`
	Severity        InterpretationSeverity     `xml:"severity"`
	Statement       InterpretationStatement    `xml:"statement"`
}

type InterpretationSeverity struct {
	Code string `xml:"code,attr"`
	ID   string `xml:"id,attr"`
	Text string `xml:",chardata"`
}

type InterpretationStatement struct {
	LeftStatement  string `xml:"leftstatement"`
	RightStatement string `xml:"rightstatement"`
}

type InterpretationMeasurements struct {
	HeartRate string `xml:"heartrate"`
}

type Waveforms struct {
	ParsedWaveforms ParsedWaveforms `xml:"parsedwaveforms"`
	RepBeats        RepBeats        `xml:"repbeats"`
}

type ParsedWaveforms struct {
	CompressFlag   string `xml:"compressflag,attr"`
	CompressMethod string `xml:"compressmethod,attr"`
	DataEncoding   string `xml:"dataencoding,attr"`
	Data           string `xml:",chardata"`
}

type RepBeats struct {
	RepBeat []RepBeat `xml:"repbeat"`
}

type RepBeat struct {
	LeadName string `xml:"leadname,attr"`
	Duration string `xml:"duration,attr"`
	Data     string `xml:",chardata"`
}

package mindraytofda

import "time"

type MindrayData struct {
	Patient     PatientData
	Device      DeviceData
	Record      RecordParams
	Leads       map[string][]int
	Measurement MeasurementData
}

type PatientData struct {
	Name      string
	PatientID string
	Gender    string // "M", "F", "UN"
	Paced     bool
	Location  string
	StartTime time.Time
	EndTime   time.Time
}

type DeviceData struct {
	SerialNumber string
	SoftwareName string
	ModelName    string
	Manufacturer string
}

type RecordParams struct {
	SampleRate int
	Scale      float64 // µV per digit (1.0 for Mindray — data is already µV)
}

type MeasurementData struct {
	HeartRate   int
	PRInterval  int
	QRSDuration int
	QTInterval  int
	QTcInterval int
	PAxis       int
	QRSAxis     int
	TAxis       int
	HasPAxis    bool
	HasQRSAxis  bool
	HasTAxis    bool
}

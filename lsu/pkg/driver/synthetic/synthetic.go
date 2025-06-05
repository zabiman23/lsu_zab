package synthetic

import (
	"time"

	"luf.co/lsu/pkg/driver"
)

type SyntheticDriver struct {
	tuner    SyntheticRXTuner
	id       string
	filename string
}

type SyntheticRXTuner struct {
	driver          *SyntheticDriver
	index           uint32
	centerFrequency uint32
	sampleRate      uint32
	bandwidth       uint32
	gain            uint32
	dcBias          bool
	agc             bool
}

func NewDriver(sensorID string, file string) *SyntheticDriver {
	return &SyntheticDriver{
		id:       sensorID,
		filename: file,
	}
}

// Init initializes the B210 driver
func (d *SyntheticDriver) Init(args []string) error {
	d.tuner = SyntheticRXTuner{
		driver: d,
		index:  0,
	}

	d.tuner.Start()
	return nil
}

// Close closes the B210 driver
func (d *SyntheticDriver) Close() error {
	d.tuner.Stop()
	return nil
}

// GetNumTuners returns the number of tuners
func (d *SyntheticDriver) GetNumTuners() uint32 {
	return 1
}

// GetTuningGranularity returns the tuning granularity
func (d *SyntheticDriver) GetTuningGranularity() driver.TuningGranularity {
	return driver.TuningGranularity{
		BoundaryHz: 1.0,
		Precision:  1.0,
	}
}

// GetStatus returns the status of the SDR
func (d *SyntheticDriver) GetStatus() driver.SDRStatus {
	return driver.SDRStatus{
		Alive: true,
		Position: driver.Location{
			Latitude:  float32(39.5489230),
			Longitude: float32(-76.0905200),
			Elevation: float32(100),
		},
	}
}

// GetTuners returns the tuners
func (d *SyntheticDriver) GetTuners() []driver.SDRTuner {
	tuners := make([]driver.SDRTuner, 1)
	tuners[0] = &d.tuner
	return tuners
}

// GetCenterFrequency gets the center frequency
func (t *SyntheticRXTuner) GetCenterFrequency() uint32 {
	return t.centerFrequency
}

// SetCenterFrequency sets the center frequency
func (t *SyntheticRXTuner) SetCenterFrequency(freq uint32) error {
	t.centerFrequency = freq
	return nil
}

// GetUseableBandwidth gets the useable bandwidth
func (t *SyntheticRXTuner) GetUseableBandwidth() uint32 {
	return t.bandwidth
}

// GetCurrentBandwidth gets the current bandwidth
func (t *SyntheticRXTuner) GetCurrentBandwidth() uint32 {
	return t.bandwidth
}

// SetCurrentBandwidth sets the current bandwidth
func (t *SyntheticRXTuner) SetCurrentBandwidth(bandwidth uint32) error {
	t.bandwidth = bandwidth
	return nil
}

// GetSampleRate gets the sample rate
func (t *SyntheticRXTuner) GetSampleRate() uint32 {
	return t.sampleRate
}

// SetSampleRate sets the sample rate
func (t *SyntheticRXTuner) SetSampleRate(rate uint32) error {
	t.sampleRate = rate
	return nil
}

// GetGain gets the gain
func (t *SyntheticRXTuner) GetGain() uint32 {
	return t.gain
}

// SetGain sets the gain
func (t *SyntheticRXTuner) SetGain(gain uint32) error {
	t.gain = gain
	return nil
}

// GetDcBias gets the DC bias setting
func (t *SyntheticRXTuner) GetDcBias() bool {
	return t.dcBias
}

// SetDcBias sets the DC bias setting
func (t *SyntheticRXTuner) SetDcBias(dcBias bool) error {
	t.dcBias = dcBias
	return nil
}

// GetAgc gets the AGC setting
func (t *SyntheticRXTuner) GetAgc() bool {
	return t.agc
}

// SetAgc sets the AGC setting
func (t *SyntheticRXTuner) SetAgc(agc bool) error {
	t.agc = agc
	return nil
}

// Start starts the tuner
func (t *SyntheticRXTuner) Start() {

}

// Stop stops the tuner
func (t *SyntheticRXTuner) Stop() {

}

func (t *SyntheticRXTuner) GetVisualizationData() driver.SpectralDataSlice {
	size := t.GetPowerDbsSize()
	data := t.GetPowerDbsData()

	timestamp := time.Now().UnixNano()
	return driver.SpectralDataSlice{
		TimestampNs:  timestamp,
		ChannelIndex: int(t.index),
		CenterFreqHz: t.centerFrequency,
		SampleRateHz: t.sampleRate,
		FftSize:      int(size),
		PowerDb:      data,
		FreqMinHz:    -float64(t.bandwidth) / 2.0,
		FreqMaxHz:    float64(t.bandwidth) / 2.0,
	}
}

func (t *SyntheticRXTuner) GetPowerDbsSize() uint32 {
	// For synthetic data, we can return a fixed size
	return 8192 // Example size, adjust as needed
}

func (t *SyntheticRXTuner) GetPowerDbsData() []float32 {
	// Generate synthetic power data for visualization
	size := t.GetPowerDbsSize()
	data := make([]float32, size)

	// Calculate the target dB value based on time ramping from -100 to -10 over 5 seconds
	cycleDurationNs := int64(5 * time.Second)
	currentTimeNs := time.Now().UnixNano()
	timeInCycleNs := currentTimeNs % cycleDurationNs

	// Calculate progress within the 5-second cycle (0.0 to 1.0)
	progress := float64(timeInCycleNs) / float64(cycleDurationNs)

	startDb := -100.0
	endDb := -10.0
	dbRange := endDb - startDb // 90.0

	// Linearly interpolate the dB value based on progress
	currentDb := float32(startDb + dbRange*progress)

	// Fill the entire data slice with the calculated time-dependent dB value
	for i := range data {
		data[i] = currentDb
	}

	return data
}

package driver

type TuningGranularity struct {
	BoundaryHz float32
	Precision  float32
}

type Location struct {
	Latitude  float32
	Longitude float32
	Elevation float32
}

type SDRStatus struct {
	Alive    bool
	Position Location
	Coherent bool
}

type SpectralDataSlice struct {
	TimestampNs  int64     `json:"ts"`      // Nanosecond timestamp for the start of this FFT window
	ChannelIndex int       `json:"chan"`    // Which SDR channel this data is from
	CenterFreqHz uint32    `json:"cfHz"`    // Center frequency the tuner was set to for this slice
	SampleRateHz uint32    `json:"fsHz"`    // Sample rate used for this slice
	FftSize      int       `json:"fftSize"` // Number of points in the FFT (and the output array)
	PowerDb      []float32 `json:"powerDb"` // FFT-SHIFTED array of log power values (dB scale)
	FreqMinHz    float64   `json:"fMinHz"`  // Frequency corresponding to the FIRST element in powerDb (after shift, usually -Fs/2)
	FreqMaxHz    float64   `json:"fMaxHz"`  // Frequency corresponding to the LAST element in powerDb (after shift, usually +Fs/2)
}

type SDRTuner interface {
	GetCenterFrequency() uint32
	SetCenterFrequency(freq uint32) error
	GetUseableBandwidth() uint32
	GetCurrentBandwidth() uint32
	SetCurrentBandwidth(bandwidth uint32) error
	GetSampleRate() uint32
	SetSampleRate(rate uint32) error
	GetGain() uint32
	SetGain(gain uint32) error
	GetDcBias() bool
	SetDcBias(dcBias bool) error
	GetAgc() bool
	SetAgc(agc bool) error
	Start()
	Stop()
	GetVisualizationData() SpectralDataSlice
}

type SDRDriver interface {
	Init(args []string) error
	Close() error
	GetNumTuners() uint32
	GetTuningGranularity() TuningGranularity
	GetStatus() SDRStatus
	GetTuners() []SDRTuner
}

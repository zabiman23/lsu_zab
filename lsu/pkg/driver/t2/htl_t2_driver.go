package t2

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"luf.co/lsu/pkg/driver"
)

var logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
	AddSource: true,
	Level:     slog.LevelDebug,
}))

// HTLT2Driver implements the SDRDriver interface for the HTL-T2 SDR.
type HTLT2Driver struct {
	ip             string
	port           int
	tuners         []HTLT2RXTuner // Collection of tuners in T2
	coherentMode   bool
	coherentTuners []uint32 // Groups of tuner indices that are coherent
	id             string
}

type HTLT2RXTuner struct {
	// Exported fields in HTLT2RXTuner to allow access from other packages.
	SIp            string
	SPort          uint32
	SMacAddr       string
	driver         *HTLT2Driver
	index          uint32
	isActive       bool
	collectorState DdcCollector
}

// NewHTLT2Driver creates a new instance of HTLT2Driver.
func NewHTLT2Driver(sensorID string) *HTLT2Driver {
	return &HTLT2Driver{
		id: sensorID,
	}
}

// Init initializes the HTL-T2 driver.
func (d *HTLT2Driver) Init(args []string) error {
	logger.Debug("HtlT2Driver Init() called.")

	// Parse IP, port from args
	if len(args) < 2 {
		return fmt.Errorf("missing required arguments: ip, port, HostName, and LocalPort")
	}

	d.ip = args[0]
	port, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid port argument: %v", err)
	}
	d.port = port
	coreInfo, err := GetCoreInfo(fmt.Sprintf("http://%s:%d/core/info/v1", d.ip, d.port))
	if err != nil {
		return fmt.Errorf("failed to fetch core info: %v", err)
	}

	d.coherentMode = len(coreInfo.CoherencyGroups) > 0
	d.coherentTuners = make([]uint32, len(coreInfo.CoherencyGroups))
	// Store all tuner IDs for each coherency group
	d.coherentTuners = nil
	for _, group := range coreInfo.CoherencyGroups {
		for _, tid := range group.TunerIDs {
			d.coherentTuners = append(d.coherentTuners, uint32(tid))
		}
	}

	numTuners := d.GetNumTuners()
	d.tuners = make([]HTLT2RXTuner, int(numTuners))
	for i := uint32(0); i < numTuners; i++ {
		d.tuners[i] = HTLT2RXTuner{
			driver: d,
			index:  i,
		}
	}
	return nil
}

// Close cleans up resources used by the driver.
func (d *HTLT2Driver) Close() error {
	// Cleanup logic (if any)
	return nil
}

// GetNumTuners returns the number of tuners available.
func (d *HTLT2Driver) GetNumTuners() uint32 {
	logger.Debug("HTLT2Driver.GetNumTuners() called")
	ddcCaps, err := GetDdcCapabilities(fmt.Sprintf("http://%s:%d/ddc/v1/capabilities", d.ip, d.port))
	if err != nil {
		logger.Error("Failed to fetch DDC capabilities in GetNumTuners:", slog.String("error", err.Error()))
		return 0
	}
	numTuners := uint32(ddcCaps.NumTuners)
	return numTuners
}

// GetTuningGranularity returns the tuning granularity of the SDR.
func (d *HTLT2Driver) GetTuningGranularity() driver.TuningGranularity {
	logger.Debug("HTLT2Driver.GetTuningGranularity() called")
	// TODO: Replace with actual logic if needed
	return driver.TuningGranularity{
		BoundaryHz: 1.0,
		Precision:  0.1,
	}
}

// GetStatus returns the current status of the SDR.
func (d *HTLT2Driver) GetStatus() driver.SDRStatus {
	logger.Debug("HTLT2Driver.GetStatus() called")
	posStatus, err := GetCoreInfo(fmt.Sprintf("http://%s:%d/core/info/v1", d.ip, d.port))
	if err != nil {
		logger.Error("Failed to fetch core info in GetStatus:", slog.Any("error", err))
		return driver.SDRStatus{
			Alive: false,
		}
	}
	return driver.SDRStatus{
		Alive: true,
		Position: driver.Location{
			Latitude:  float32(posStatus.UnitStatus.TimePositionStatus.PositionStatus.Latitude),
			Longitude: float32(posStatus.UnitStatus.TimePositionStatus.PositionStatus.Longitude),
			Elevation: float32(posStatus.UnitStatus.TimePositionStatus.PositionStatus.Altitude),
		},
		Coherent: d.coherentMode,
	}
}

// GetTuners returns the tuners
func (d *HTLT2Driver) GetTuners() []driver.SDRTuner {
	logger.Debug("HTLT2Driver.GetTuners() called")
	tuners := make([]driver.SDRTuner, len(d.tuners))
	for i := range d.tuners {
		tuners[i] = &d.tuners[i]
	}
	return tuners
}

func (t *HTLT2RXTuner) GetCenterFrequency() uint32 {
	logger.Debug("HTLT2RXTuner.GetCenterFrequency() called")
	if err := t.validateActive(); err != nil {
		return 0
	}
	return uint32(t.collectorState.CenterFreqHz)
}

func (t *HTLT2RXTuner) SetCenterFrequency(freq uint32) error {
	logger.Debug("HTLT2RXTuner.SetCenterFrequency() called")
	if err := t.validateActive(); err != nil {
		return err
	}
	cfg := t.collectorState
	cfg.CenterFreqHz = uint64(freq)

	if err := PostCollectorConfig(fmt.Sprintf("http://%s:%d/ddc/v1/collector", t.driver.ip, t.driver.port), cfg); err != nil {
		return fmt.Errorf("failed to post center frequency configuration: %v", err)
	}
	t.collectorState = cfg
	return nil
}

func (t *HTLT2RXTuner) GetUseableBandwidth() uint32 {
	logger.Debug("HTLT2RXTuner.GetUseableBandwidth() called")
	return uint32(t.collectorState.BandwidthHz)
}

func (t *HTLT2RXTuner) GetCurrentBandwidth() uint32 {
	logger.Debug("HTLT2RXTuner.GetCurrentBandwidth() called")
	if err := t.validateActive(); err != nil {
		return 0
	}
	return uint32(t.collectorState.BandwidthHz)
}

func (t *HTLT2RXTuner) SetCurrentBandwidth(bandwidth uint32) error {
	logger.Debug("HTLT2RXTuner.SetCurrentBandwidth() called")
	if err := t.validateActive(); err != nil {
		return err
	}
	cfg := t.collectorState
	cfg.BandwidthHz = uint64(bandwidth)

	if err := PostCollectorConfig(fmt.Sprintf("http://%s:%d/ddc/v1/collector", t.driver.ip, t.driver.port), cfg); err != nil {
		return fmt.Errorf("failed to post bandwidth configuration: %v", err)
	}
	t.collectorState = cfg
	return nil
}

func (t *HTLT2RXTuner) GetSampleRate() uint32 {
	logger.Debug("HTLT2RXTuner.GetSampleRate() called")
	if err := t.validateActive(); err != nil {
		return 0
	}
	sampleRateFloat64, err := t.collectorState.SampleRateSps.Float64() // Convert the sample rate from JSON-compatible number format to a float64 for internal use.
	if err != nil {
		logger.Error("Error converting sample rate:", slog.Any("error", err))
		return 0
	}
	return uint32(sampleRateFloat64)
}

func (t *HTLT2RXTuner) SetSampleRate(sr uint32) error {
	logger.Debug("HTLT2RXTuner.SetSampleRate() called")
	if err := t.validateActive(); err != nil {
		return err
	}
	cfg := t.collectorState
	cfg.SampleRateSps = json.Number(strconv.FormatUint(uint64(sr), 10)) // Convert the sample rate to a JSON-compatible number format for serialization.
	if err := PostCollectorConfig(fmt.Sprintf("http://%s:%d/ddc/v1/collector", t.driver.ip, t.driver.port), cfg); err != nil {
		return fmt.Errorf("failed to post sample rate configuration: %v", err)
	}
	t.collectorState = cfg
	return nil
}

func (t *HTLT2RXTuner) GetGain() uint32 {
	logger.Debug("HTLT2RXTuner.GetGain() called")
	if err := t.validateActive(); err != nil {
		return 0
	}
	return uint32(t.collectorState.AGCSettings.Attenuation_dB)
}

func (t *HTLT2RXTuner) SetGain(gain uint32) error {
	logger.Debug("HTLT2RXTuner.SetGain() called")
	if err := t.validateActive(); err != nil {
		return err
	}
	cfg := t.collectorState
	cfg.AGCSettings.Attenuation_dB = float64(gain)
	if err := PostCollectorConfig(fmt.Sprintf("http://%s:%d/ddc/v1/collector", t.driver.ip, t.driver.port), cfg); err != nil {
		return fmt.Errorf("failed to post gain configuration: %v", err)
	}
	t.collectorState = cfg
	return nil
}

func (t *HTLT2RXTuner) GetDcBias() bool {
	logger.Debug("HTLT2RXTuner.GetDcBias() called")
	if err := t.validateActive(); err != nil {
		return false
	}
	return t.collectorState.EnablePreamp
}

func (t *HTLT2RXTuner) SetDcBias(enabled bool) error {
	logger.Debug("HTLT2RXTuner.SetDcBias() called")
	cfg := t.collectorState
	cfg.EnablePreamp = enabled
	if err := PostCollectorConfig(fmt.Sprintf("http://%s:%d/ddc/v1/collector", t.driver.ip, t.driver.port), cfg); err != nil {
		return fmt.Errorf("failed to post dc bias configuration: %v", err)
	}
	t.collectorState = cfg
	return nil
}

func (t *HTLT2RXTuner) GetAgc() bool {
	logger.Debug("HTLT2RXTuner.GetAgc() called")
	return t.collectorState.AGCSettings.Enabled
}

func (t *HTLT2RXTuner) SetAgc(enabled bool) error {
	logger.Debug("HTLT2RXTuner.SetAgc() called")
	if err := t.validateActive(); err != nil {
		return err
	}
	cfg := t.collectorState
	cfg.AGCSettings.Enabled = enabled
	if err := PostCollectorConfig(fmt.Sprintf("http://%s:%d/ddc/v1/collector", t.driver.ip, t.driver.port), cfg); err != nil {
		return fmt.Errorf("failed to post agc configuration: %v", err)
	}
	t.collectorState = cfg
	return nil
}

func (t *HTLT2RXTuner) Start() {
	logger.Debug("HTLT2RXTuner.Start() called")

	if t.SIp != "" && t.SPort != 0 && t.SMacAddr != "" {
		logger.Debug("HTLT2RXTuner.Start() using runtime configuration")
		t.collectorState = NewDdcCollector(t.SIp, t.SPort, t.SMacAddr, t.index)
	} else {
		logger.Debug("HTLT2RXTuner.Start() using default configuration")
		t.collectorState = NewDdcCollector("127.0.0.1", 0, "11:22:33:44:55:66", t.index) //config the collector with default ip, port and mac addr if the user does not provide them
	}

	// Send the configuration to the REST endpoint
	err := PostCollectorConfig(fmt.Sprintf("http://%s:%d/ddc/v1/collector", t.driver.ip, t.driver.port), t.collectorState)
	if err != nil {
		logger.Error("Failed to initialize tuner", slog.Uint64("tuner_index", uint64(t.index)), slog.Any("error", err))
		return
	}

	if !t.isActive {
		t.isActive = true
	}
	logger.Debug("HTLT2RXTuner.Start() completed", slog.Uint64("TunerID", uint64(t.index)), slog.Bool("Status", t.isActive))
}

func (t *HTLT2RXTuner) Stop() {
	logger.Debug("HTLT2RXTuner.Stop() called", slog.Uint64("TunerID", uint64(t.index)), slog.Bool("isActive", t.isActive))
	if t.isActive {
		// Construct the URL for stopping the tuner
		url := fmt.Sprintf("http://%s:%d/ddc/v1/collector/%d", t.driver.ip, t.driver.port, t.index)
		logger.Debug("Sending request to stop tuner", slog.String("url", url))

		// Call DeleteCollectorConfig to stop the tuner
		success := DeleteCollectorConfig(url, t.collectorState, t.index)

		if !success {
			logger.Error("Failed to stop tuner. Please check the configuration.", slog.Uint64("tuner_index", uint64(t.index)))
			return
		}
		// Mark the tuner as inactive
		t.isActive = false
		logger.Debug("Tuner successfully stopped.", slog.Uint64("tuner_index", uint64(t.index)))
	}
}

// TODO: Replace with actual logic and return the appropriate type.
func (t *HTLT2RXTuner) GetVisualizationData() driver.SpectralDataSlice {
	logger.Debug("HTLT2RXTuner.GetVisualizationData() called")
	return driver.SpectralDataSlice{}
}

func (d *HTLT2Driver) StopAllTuners() {
	logger.Debug("HTLT2Driver.StopAllTuners() called")
	for i := range d.tuners {
		tuner := &d.tuners[i]
		if tuner.isActive {
			DeleteCollectorConfig(fmt.Sprintf("http://%s:%d/ddc/v1/collector", d.ip, d.port), tuner.collectorState, tuner.index)
			tuner.isActive = false
		}
	}
}

// Utility function to check if the tuner is active before performing any operations.
func (t *HTLT2RXTuner) validateActive() error {
	logger.Debug("HTLT2RXTuner.validateActive() called")
	if !t.isActive {
		// In Go, error message should start with a lowercase letter, per convention.
		return fmt.Errorf("tuner %d is not active; please configure the tuner first", t.index)
	}
	return nil
}

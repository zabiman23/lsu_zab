package x40

import (
	"fmt"
	"log/slog"
	"os"
	"time"
	"unsafe"

	"luf.co/lsu/pkg/driver"
)

// #cgo CFLAGS: -I.
// #cgo LDFLAGS: -L./build -L/usr/local/cuda/lib64 -lx40_driver -lcufft -lcudart  -lstdc++
// #include "cpp/x40_driver.h"
// #include <stdlib.h>
// #include <stdint.h>
// #include <stdbool.h>
import "C"

// X40Driver is a driver for UHD based SDRs
type X40Driver struct {
	handle       C.X40DriverHandle
	tuners       []X40RXTuner
	coherentMode bool
	id           string
}

type X40RXTuner struct {
	driver *X40Driver
	index  uint32
}

var logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

func NewX40Driver(sensorID string) *X40Driver {
	return &X40Driver{
		id: sensorID,
	}
}

// Init initializes the UHD driver
func (d *X40Driver) Init(args []string) error {
	logger.Debug("X40Driver.Init() called")

	cArgs := C.CString(args[0]) // Assuming args has at least one element.
	defer C.free(unsafe.Pointer(cArgs))

	d.handle = C.X40Driver_Init(cArgs)
	if d.handle == nil {
		return fmt.Errorf("X40Driver_Init failed")
	}

	numTuners := C.X40Driver_GetNumTuners(d.handle)
	d.tuners = make([]X40RXTuner, numTuners)
	for i := uint32(0); i < uint32(numTuners); i++ {
		d.tuners[i] = X40RXTuner{driver: d, index: i}
	}

	return nil
}

// Close closes the UHD driver
func (d *X40Driver) Close() error {
	logger.Debug("X40Driver.Close() called")
	C.X40Driver_Close(d.handle)
	return nil
}

// GetNumTuners returns the number of tuners
func (d *X40Driver) GetNumTuners() uint32 {
	return uint32(len(d.tuners))
}

// GetTuningGranularity returns the tuning granularity
func (d *X40Driver) GetTuningGranularity() driver.TuningGranularity {
	// TODO: Implement this
	return driver.TuningGranularity{
		BoundaryHz: 1.0,
		Precision:  0.1,
	}
}

// GetStatus returns the status of the SDR
func (d *X40Driver) GetStatus() driver.SDRStatus {
	// TODO: Implement this
	return driver.SDRStatus{
		Alive: true,
		Position: driver.Location{
			Latitude:  float32(C.X40Driver_GetLatitude(d.handle)),
			Longitude: float32(C.X40Driver_GetLongitude(d.handle)),
			Elevation: float32(C.X40Driver_GetElevation(d.handle)),
		},
	}
}

// GetTuners returns the tuners
func (d *X40Driver) GetTuners() []driver.SDRTuner {
	tuners := make([]driver.SDRTuner, len(d.tuners))
	for i, tuner := range d.tuners {
		tuners[i] = tuner
	}
	return tuners
}

// GetCenterFrequency gets the center frequency
func (t X40RXTuner) GetCenterFrequency() uint32 {
	logger.Debug("X40RXTuner.GetCenterFrequency() called")
	freq := C.X40Driver_GetRxFrequency(t.driver.handle, C.uint32_t(t.index))
	return uint32(freq)
}

// SetCenterFrequency sets the center frequency
func (t X40RXTuner) SetCenterFrequency(freq uint32) error {
	logger.Debug("X40RXTuner.SetCenterFrequency() called with freq: %d", freq)
	C.X40Driver_SetRxFrequency(t.driver.handle, C.uint32_t(t.index), C.uint32_t(freq))
	return nil
}

// GetUseableBandwidth gets the useable bandwidth
func (t X40RXTuner) GetUseableBandwidth() uint32 {
	// TODO: Implement this
	return 0
}

// GetCurrentBandwidth gets the current bandwidth
func (t X40RXTuner) GetCurrentBandwidth() uint32 {
	logger.Debug("X40RXTuner.GetCurrentBandwidth() called")
	bandwidth := C.X40Driver_GetRxBandwidth(t.driver.handle, C.uint32_t(t.index))
	return uint32(bandwidth)
}

// SetCurrentBandwidth sets the current bandwidth
func (t X40RXTuner) SetCurrentBandwidth(bandwidth uint32) error {
	logger.Debug("X40RXTuner.SetCurrentBandwidth() called with bandwidth: %d", bandwidth)
	C.X40Driver_SetRxBandwidth(t.driver.handle, C.uint32_t(t.index), C.uint32_t(bandwidth))
	return nil
}

// GetSampleRate gets the sample rate
func (t X40RXTuner) GetSampleRate() uint32 {
	logger.Debug("X40RXTuner.GetSampleRate() called")
	rate := C.X40Driver_GetSampleRate(t.driver.handle, C.uint32_t(t.index))
	return uint32(rate)
}

// SetSampleRate sets the sample rate
func (t X40RXTuner) SetSampleRate(rate uint32) error {
	logger.Debug("X40RXTuner.SetSampleRate() called with rate: %d", rate)
	C.X40Driver_SetSampleRate(t.driver.handle, C.uint32_t(t.index), C.uint32_t(rate))
	return nil
}

// GetGain gets the gain
func (t X40RXTuner) GetGain() uint32 {
	logger.Debug("X40RXTuner.GetGain() called")
	gain := C.X40Driver_GetGain(t.driver.handle, C.uint32_t(t.index))
	return uint32(gain)
}

// SetGain sets the gain
func (t X40RXTuner) SetGain(gain uint32) error {
	logger.Debug("X40RXTuner.SetGain() called with gain: %d", gain)
	C.X40Driver_SetGain(t.driver.handle, C.uint32_t(t.index), C.uint32_t(gain))
	return nil
}

// GetDcBias gets the DC bias setting
func (t X40RXTuner) GetDcBias() bool {
	logger.Debug("X40RXTuner.GetDcBias() called")
	dcBias := C.X40Driver_GetDcBias(t.driver.handle, C.uint32_t(t.index))
	return bool(dcBias)
}

// SetDcBias sets the DC bias setting
func (t X40RXTuner) SetDcBias(dcBias bool) error {
	logger.Debug("X40RXTuner.SetDcBias() called with dcBias: %t", dcBias)
	C.X40Driver_SetDcBias(t.driver.handle, C.uint32_t(t.index), C.bool(dcBias))
	return nil
}

// GetAgc gets the AGC setting
func (t X40RXTuner) GetAgc() bool {
	logger.Debug("X40RXTuner.GetAgc() called")
	agc := C.X40Driver_GetAgc(t.driver.handle, C.uint32_t(t.index))
	return bool(agc)
}

// SetAgc sets the AGC setting
func (t X40RXTuner) SetAgc(agc bool) error {
	logger.Debug("X40RXTuner.SetAgc() called with agc: %t", agc)
	C.X40Driver_SetAgc(t.driver.handle, C.uint32_t(t.index), C.bool(agc))
	return nil
}

// Start starts the tuner
func (t X40RXTuner) Start() {
	logger.Debug("X40RXTuner.Start() called")
	C.X40Driver_StartRxStream(t.driver.handle, C.uint32_t(t.index))
}

// Stop stops the tuner
func (t X40RXTuner) Stop() {
	logger.Debug("X40RXTuner.Stop() called")
	C.X40Driver_StopRxStream(t.driver.handle, C.uint32_t(t.index))
}

func (t X40RXTuner) GetVisualizationData() driver.SpectralDataSlice {
	size := C.X40Driver_GetPowerDbsSize(t.driver.handle, C.uint32_t(t.index))
	data := make([]float32, size)

	C.X40Driver_GetPowerDbsData(t.driver.handle, C.uint32_t(t.index), (*C.float)(&data[0]), C.uint32_t(size))

	timestamp := time.Now().UnixNano()
	return driver.SpectralDataSlice{
		TimestampNs:  timestamp,
		ChannelIndex: int(t.index),
		CenterFreqHz: uint32(C.X40Driver_GetRxFrequency(t.driver.handle, C.uint32_t(t.index))),
		SampleRateHz: uint32(C.X40Driver_GetSampleRate(t.driver.handle, C.uint32_t(t.index))),
		FftSize:      int(size),
		PowerDb:      data,
		FreqMinHz:    float64(-C.X40Driver_GetRxBandwidth(t.driver.handle, C.uint32_t(t.index)) / 2.0),
		FreqMaxHz:    float64(C.X40Driver_GetRxBandwidth(t.driver.handle, C.uint32_t(t.index)) / 2.0),
	}
}

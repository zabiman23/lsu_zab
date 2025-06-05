package b210

import (
	"fmt"
	"time"
	"unsafe"

	"luf.co/lsu/pkg/driver"
)

// #cgo CFLAGS: -I. -I/usr/local/cuda-12.6/targets/aarch64-linux/include
// #cgo CXXFLAGS: -I. -I/usr/local/cuda-12.6/targets/aarch64-linux/include
// #cgo LDFLAGS: -L. -L./build -L/usr/local/cuda-12.6/targets/aarch64-linux/lib -lb210_driver -luhd -lcufft -lcudart  -lstdc++
// #include "b210_driver.h"
// #include <stdlib.h>
// #include <stdint.h>
// #include <stdbool.h>
import "C"

type B210Driver struct {
	handle       C.B210DriverHandle
	tuners       []B210RXTuner
	coherentMode bool
	id           string
}

type B210RXTuner struct {
	driver *B210Driver
	index  uint32
}

func NewB210Driver(sensorID string) *B210Driver {
	return &B210Driver{
		id: sensorID,
	}
}

// Init initializes the B210 driver
func (d *B210Driver) Init(args []string) error {
	cArgs := C.CString(args[0]) // Assuming args has at least one element.
	defer C.free(unsafe.Pointer(cArgs))

	d.handle = C.B210Driver_Init(cArgs)
	if d.handle == nil {
		return fmt.Errorf("B210Driver_Init failed")
	}

	numTuners := C.B210Driver_GetNumTuners(d.handle)
	d.tuners = make([]B210RXTuner, numTuners)
	for i := uint32(0); i < uint32(numTuners); i++ {
		d.tuners[i] = B210RXTuner{driver: d, index: i}
	}

	// Start the RX stream
	C.B210Driver_SetGain(d.handle, C.uint32_t(0), C.uint32_t(10)) // Set initial gain to 10
	C.B210Driver_StartRxStream(d.handle, C.uint32_t(0))

	return nil
}

// Close closes the B210 driver
func (d *B210Driver) Close() error {
	C.B210Driver_Close(d.handle)
	return nil
}

// GetNumTuners returns the number of tuners
func (d *B210Driver) GetNumTuners() uint32 {
	return uint32(len(d.tuners))
}

// GetTuningGranularity returns the tuning granularity
func (d *B210Driver) GetTuningGranularity() driver.TuningGranularity {
	// TODO: Implement this
	return driver.TuningGranularity{
		BoundaryHz: 1.0,
		Precision:  0.1,
	}
}

// GetStatus returns the status of the SDR
func (d *B210Driver) GetStatus() driver.SDRStatus {
	return driver.SDRStatus{
		Alive: true,
		Position: driver.Location{
			Latitude:  float32(C.B210Driver_GetLatitude(d.handle)),
			Longitude: float32(C.B210Driver_GetLongitude(d.handle)),
			Elevation: float32(C.B210Driver_GetElevation(d.handle)),
		},
	}
}

// GetTuners returns the tuners
func (d *B210Driver) GetTuners() []driver.SDRTuner {
	tuners := make([]driver.SDRTuner, len(d.tuners))
	for i, tuner := range d.tuners {
		tuners[i] = tuner
	}
	return tuners
}

// GetCenterFrequency gets the center frequency
func (t B210RXTuner) GetCenterFrequency() uint32 {
	freq := C.B210Driver_GetRxFrequency(t.driver.handle, C.uint32_t(t.index))
	return uint32(freq)
}

// SetCenterFrequency sets the center frequency
func (t B210RXTuner) SetCenterFrequency(freq uint32) error {
	fmt.Printf("B210RXTuner.SetCenterFrequency() tuner id (%d) called with freq: %d\n", t.index, freq)
	C.B210Driver_SetRxFrequency(t.driver.handle, C.uint32_t(t.index), C.uint32_t(freq))
	return nil
}

// GetUseableBandwidth gets the useable bandwidth
func (t B210RXTuner) GetUseableBandwidth() uint32 {
	// TODO: Implement this
	return 0
}

// GetCurrentBandwidth gets the current bandwidth
func (t B210RXTuner) GetCurrentBandwidth() uint32 {
	bandwidth := C.B210Driver_GetRxBandwidth(t.driver.handle, C.uint32_t(t.index))
	return uint32(bandwidth)
}

// SetCurrentBandwidth sets the current bandwidth
func (t B210RXTuner) SetCurrentBandwidth(bandwidth uint32) error {
	fmt.Printf("B210RXTuner.SetCurrentBandwidth() called with bandwidth: %d\n", bandwidth)
	C.B210Driver_SetRxBandwidth(t.driver.handle, C.uint32_t(t.index), C.uint32_t(bandwidth))
	return nil
}

// GetSampleRate gets the sample rate
func (t B210RXTuner) GetSampleRate() uint32 {
	rate := C.B210Driver_GetSampleRate(t.driver.handle, C.uint32_t(t.index))
	return uint32(rate)
}

// SetSampleRate sets the sample rate
func (t B210RXTuner) SetSampleRate(rate uint32) error {
	fmt.Printf("B210RXTuner.SetSampleRate() called with rate: %d\n", rate)
	C.B210Driver_SetSampleRate(t.driver.handle, C.uint32_t(t.index), C.uint32_t(rate))
	return nil
}

// GetGain gets the gain
func (t B210RXTuner) GetGain() uint32 {
	gain := C.B210Driver_GetGain(t.driver.handle, C.uint32_t(t.index))
	return uint32(gain)
}

// SetGain sets the gain
func (t B210RXTuner) SetGain(gain uint32) error {
	fmt.Printf("B210RXTuner.SetGain() called with gain: %d\n", gain)
	C.B210Driver_SetGain(t.driver.handle, C.uint32_t(t.index), C.uint32_t(gain))
	return nil
}

// GetDcBias gets the DC bias setting
func (t B210RXTuner) GetDcBias() bool {
	dcBias := C.B210Driver_GetDcBias(t.driver.handle, C.uint32_t(t.index))
	return bool(dcBias)
}

// SetDcBias sets the DC bias setting
func (t B210RXTuner) SetDcBias(dcBias bool) error {
	fmt.Printf("B210RXTuner.SetDcBias() called with dcBias: %t\n", dcBias)
	C.B210Driver_SetDcBias(t.driver.handle, C.uint32_t(t.index), C.bool(dcBias))
	return nil
}

// GetAgc gets the AGC setting
func (t B210RXTuner) GetAgc() bool {
	agc := C.B210Driver_GetAgc(t.driver.handle, C.uint32_t(t.index))
	return bool(agc)
}

// SetAgc sets the AGC setting
func (t B210RXTuner) SetAgc(agc bool) error {
	fmt.Printf("B210RXTuner.SetAgc() called with agc: %t\n", agc)
	C.B210Driver_SetAgc(t.driver.handle, C.uint32_t(t.index), C.bool(agc))
	return nil
}

// Start starts the tuner
func (t B210RXTuner) Start() {
	fmt.Println("B210RXTuner.Start() called")
	C.B210Driver_StartRxStream(t.driver.handle, C.uint32_t(t.index))
}

// Stop stops the tuner
func (t B210RXTuner) Stop() {
	fmt.Println("B210RXTuner.Stop() called")
	C.B210Driver_StopRxStream(t.driver.handle, C.uint32_t(t.index))
}

func (t B210RXTuner) GetVisualizationData() driver.SpectralDataSlice {
	size := C.B210Driver_GetPowerDbsSize(t.driver.handle, C.uint32_t(t.index))
	data := make([]float32, size)

	C.B210Driver_GetPowerDbsData(t.driver.handle, C.uint32_t(t.index), (*C.float)(&data[0]), C.uint32_t(size))

	timestamp := time.Now().UnixNano()
	return driver.SpectralDataSlice{
		TimestampNs:  timestamp,
		ChannelIndex: int(t.index),
		CenterFreqHz: uint32(C.B210Driver_GetRxFrequency(t.driver.handle, C.uint32_t(t.index))),
		SampleRateHz: uint32(C.B210Driver_GetSampleRate(t.driver.handle, C.uint32_t(t.index))),
		FftSize:      int(size),
		PowerDb:      data,
		FreqMinHz:    float64(-C.B210Driver_GetRxBandwidth(t.driver.handle, C.uint32_t(t.index)) / 2.0),
		FreqMaxHz:    float64(C.B210Driver_GetRxBandwidth(t.driver.handle, C.uint32_t(t.index)) / 2.0),
	}
}

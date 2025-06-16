package x310

import (
	"fmt"
	"time"
	"unsafe"

	"luf.co/lsu/pkg/driver"
)

// #cgo CFLAGS: -I. -I/usr/local/cuda-12.6/targets/aarch64-linux/include
// #cgo CXXFLAGS: -I. -I/usr/local/cuda-12.6/targets/aarch64-linux/include
// #cgo LDFLAGS: -L. -L./build -L/usr/local/cuda-12.6/targets/aarch64-linux/lib -lx310_driver -luhd -lcufft -lcudart  -lstdc++
// #include "x310_driver.h"
// #include <stdlib.h>
// #include <stdint.h>
// #include <stdbool.h>
import "C"

type X310Driver struct {
	handle       C.X310DriverHandle
	tuners       []X310RXTuner
	coherentMode bool
	id           string
}

type X310RXTuner struct {
	driver *X310Driver
	index  uint32
}

func NewX310Driver(sensorID string) *X310Driver {
	return &X310Driver{
		id: sensorID,
	}
}

// Init initializes the X310 driver
func (d *X310Driver) Init(args []string) error {
	cArgs := C.CString(args[0]) // Assuming args has at least one element.
	defer C.free(unsafe.Pointer(cArgs))

	d.handle = C.X310Driver_Init(cArgs)
	if d.handle == nil {
		return fmt.Errorf("X310Driver_Init failed")
	}

	numTuners := C.X310Driver_GetNumTuners(d.handle)
	d.tuners = make([]X310RXTuner, numTuners)
	for i := uint32(0); i < uint32(numTuners); i++ {
		d.tuners[i] = X310RXTuner{driver: d, index: i}
	}

	// Start the RX stream
	C.X310Driver_SetGain(d.handle, C.uint32_t(0), C.uint32_t(10)) // Set initial gain to 10
	C.X310Driver_StartRxStream(d.handle, C.uint32_t(0))

	return nil
}

// Close closes the X310 driver
func (d *X310Driver) Close() error {
	C.X310Driver_Close(d.handle)
	return nil
}

// GetNumTuners returns the number of tuners
func (d *X310Driver) GetNumTuners() uint32 {
	return uint32(len(d.tuners))
}

// GetTuningGranularity returns the tuning granularity
func (d *X310Driver) GetTuningGranularity() driver.TuningGranularity {
	// TODO: Implement this
	return driver.TuningGranularity{
		BoundaryHz: 1.0,
		Precision:  0.1,
	}
}

// GetStatus returns the status of the SDR
func (d *X310Driver) GetStatus() driver.SDRStatus {
	return driver.SDRStatus{
		Alive: true,
		Position: driver.Location{
			Latitude:  float32(C.X310Driver_GetLatitude(d.handle)),
			Longitude: float32(C.X310Driver_GetLongitude(d.handle)),
			Elevation: float32(C.X310Driver_GetElevation(d.handle)),
		},
	}
}

// GetTuners returns the tuners
func (d *X310Driver) GetTuners() []driver.SDRTuner {
	tuners := make([]driver.SDRTuner, len(d.tuners))
	for i, tuner := range d.tuners {
		tuners[i] = tuner
	}
	return tuners
}

// GetCenterFrequency gets the center frequency
func (t X310RXTuner) GetCenterFrequency() uint32 {
	freq := C.X310Driver_GetRxFrequency(t.driver.handle, C.uint32_t(t.index))
	return uint32(freq)
}

// SetCenterFrequency sets the center frequency
func (t X310RXTuner) SetCenterFrequency(freq uint32) error {
	fmt.Printf("X310RXTuner.SetCenterFrequency() tuner id (%d) called with freq: %d\n", t.index, freq)
	C.X310Driver_SetRxFrequency(t.driver.handle, C.uint32_t(t.index), C.uint32_t(freq))
	return nil
}

// GetUseableBandwidth gets the useable bandwidth
func (t X310RXTuner) GetUseableBandwidth() uint32 {
	// TODO: Implement this
	return 0
}

// GetCurrentBandwidth gets the current bandwidth
func (t X310RXTuner) GetCurrentBandwidth() uint32 {
	bandwidth := C.X310Driver_GetRxBandwidth(t.driver.handle, C.uint32_t(t.index))
	return uint32(bandwidth)
}

// SetCurrentBandwidth sets the current bandwidth
func (t X310RXTuner) SetCurrentBandwidth(bandwidth uint32) error {
	fmt.Printf("X310RXTuner.SetCurrentBandwidth() called with bandwidth: %d\n", bandwidth)
	C.X310Driver_SetRxBandwidth(t.driver.handle, C.uint32_t(t.index), C.uint32_t(bandwidth))
	return nil
}

// GetSampleRate gets the sample rate
func (t X310RXTuner) GetSampleRate() uint32 {
	rate := C.X310Driver_GetSampleRate(t.driver.handle, C.uint32_t(t.index))
	return uint32(rate)
}

// SetSampleRate sets the sample rate
func (t X310RXTuner) SetSampleRate(rate uint32) error {
	fmt.Printf("X310RXTuner.SetSampleRate() called with rate: %d\n", rate)
	C.X310Driver_SetSampleRate(t.driver.handle, C.uint32_t(t.index), C.uint32_t(rate))
	return nil
}

// GetGain gets the gain
func (t X310RXTuner) GetGain() uint32 {
	gain := C.X310Driver_GetGain(t.driver.handle, C.uint32_t(t.index))
	return uint32(gain)
}

// SetGain sets the gain
func (t X310RXTuner) SetGain(gain uint32) error {
	fmt.Printf("X310RXTuner.SetGain() called with gain: %d\n", gain)
	C.X310Driver_SetGain(t.driver.handle, C.uint32_t(t.index), C.uint32_t(gain))
	return nil
}

// GetDcBias gets the DC bias setting
func (t X310RXTuner) GetDcBias() bool {
	dcBias := C.X310Driver_GetDcBias(t.driver.handle, C.uint32_t(t.index))
	return bool(dcBias)
}

// SetDcBias sets the DC bias setting
func (t X310RXTuner) SetDcBias(dcBias bool) error {
	fmt.Printf("X310RXTuner.SetDcBias() called with dcBias: %t\n", dcBias)
	C.X310Driver_SetDcBias(t.driver.handle, C.uint32_t(t.index), C.bool(dcBias))
	return nil
}

// GetAgc gets the AGC setting
func (t X310RXTuner) GetAgc() bool {
	agc := C.X310Driver_GetAgc(t.driver.handle, C.uint32_t(t.index))
	return bool(agc)
}

// SetAgc sets the AGC setting
func (t X310RXTuner) SetAgc(agc bool) error {
	fmt.Printf("X310RXTuner.SetAgc() called with agc: %t\n", agc)
	C.X310Driver_SetAgc(t.driver.handle, C.uint32_t(t.index), C.bool(agc))
	return nil
}

// Start starts the tuner
func (t X310RXTuner) Start() {
	fmt.Println("X310RXTuner.Start() called")
	C.X310Driver_StartRxStream(t.driver.handle, C.uint32_t(t.index))
}

// Stop stops the tuner
func (t X310RXTuner) Stop() {
	fmt.Println("X310RXTuner.Stop() called")
	C.X310Driver_StopRxStream(t.driver.handle, C.uint32_t(t.index))
}

func (t X310RXTuner) GetVisualizationData() driver.SpectralDataSlice {
  C.X310Driver_LockFFTMutex(t.driver.handle, C.uint32_t(t.index))

	size := C.X310Driver_GetPowerDbsSize(t.driver.handle, C.uint32_t(t.index))
	data := make([]float32, size)

  if (size > 0) {
    C.X310Driver_GetPowerDbsData(t.driver.handle, C.uint32_t(t.index), (*C.float)(&data[0]), C.uint32_t(size))
  }

  C.X310Driver_UnlockFFTMutex(t.driver.handle, C.uint32_t(t.index))
	timestamp := time.Now().UnixNano()

	return driver.SpectralDataSlice{
		TimestampNs:  timestamp,
		ChannelIndex: int(t.index),
		CenterFreqHz: uint32(C.X310Driver_GetRxFrequency(t.driver.handle, C.uint32_t(t.index))),
		SampleRateHz: uint32(C.X310Driver_GetSampleRate(t.driver.handle, C.uint32_t(t.index))),
		FftSize:      int(size),
		PowerDb:      data,
		FreqMinHz:    float64(-C.X310Driver_GetRxBandwidth(t.driver.handle, C.uint32_t(t.index)) / 2.0),
		FreqMaxHz:    float64(C.X310Driver_GetRxBandwidth(t.driver.handle, C.uint32_t(t.index)) / 2.0),
	}
}

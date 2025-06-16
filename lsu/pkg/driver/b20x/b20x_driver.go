package b20x

import (
	"fmt"
	"time"
	"unsafe"

	"luf.co/lsu/pkg/driver"
)

// #cgo CFLAGS: -I. -I/usr/local/cuda-12.9/targets/x86_64-linux/lib
// #cgo CXXFLAGS: -I. -I/usr/local/cuda-12.9/targets/x86_64-linux/lib
// #cgo LDFLAGS: -L. -L./build -L/usr/local/cuda-12.9/targets/x86_64-linux/lib -lb20x_driver -luhd -lcufft -lcudart -lstdc++ -lm
// #include "b20x_driver.h"
// #include <stdlib.h>
// #include <stdint.h>
// #include <stdbool.h>
import "C"

type b20xDriver struct {
	handle       C.b20xDriverHandle
	tuners       []b20xRXTuner
	coherentMode bool
	id           string
}

type b20xRXTuner struct {
	driver *b20xDriver
	index  uint32
}

func Newb20xDriver(sensorID string) *b20xDriver {
	return &b20xDriver{
		id: sensorID,
	}
}

// Init initializes the b20x driver
func (d *b20xDriver) Init(args []string) error {
	cArgs := C.CString(args[0]) // Assuming args has at least one element.
	defer C.free(unsafe.Pointer(cArgs))

	d.handle = C.b20xDriver_Init(cArgs)
	if d.handle == nil {
		return fmt.Errorf("b20xDriver_Init failed")
	}

	numTuners := C.b20xDriver_GetNumTuners(d.handle)
	d.tuners = make([]b20xRXTuner, numTuners)
	for i := uint32(0); i < uint32(numTuners); i++ {
		d.tuners[i] = b20xRXTuner{driver: d, index: i}
	}

	// Start the RX stream
	C.b20xDriver_SetGain(d.handle, C.uint32_t(0), C.uint32_t(35)) // Set initial gain to 10
	C.b20xDriver_SetSampleRate(d.handle, C.uint32_t(0), C.uint32_t(20000000)) // Set initial samp rate to 20MHz for stress test
	C.b20xDriver_SetRxFrequency(d.handle, C.uint32_t(0), C.uint32_t(100100000)) // Set initial center freq to 100.1 MHz
	C.b20xDriver_StartRxStream(d.handle, C.uint32_t(0))

	return nil
}

// Close closes the b20x driver
func (d *b20xDriver) Close() error {
	C.b20xDriver_Close(d.handle)
	return nil
}

// GetNumTuners returns the number of tuners
func (d *b20xDriver) GetNumTuners() uint32 {
	return uint32(len(d.tuners))
}

// GetTuningGranularity returns the tuning granularity
func (d *b20xDriver) GetTuningGranularity() driver.TuningGranularity {
	// TODO: Implement this
	return driver.TuningGranularity{
		BoundaryHz: 1.0,
		Precision:  0.1,
	}
}

// GetStatus returns the status of the SDR
func (d *b20xDriver) GetStatus() driver.SDRStatus {
	return driver.SDRStatus{
		Alive: true,
		Position: driver.Location{
			Latitude:  float32(C.b20xDriver_GetLatitude(d.handle)),
			Longitude: float32(C.b20xDriver_GetLongitude(d.handle)),
			Elevation: float32(C.b20xDriver_GetElevation(d.handle)),
		},
	}
}

// GetTuners returns the tuners
func (d *b20xDriver) GetTuners() []driver.SDRTuner {
	tuners := make([]driver.SDRTuner, len(d.tuners))
	for i, tuner := range d.tuners {
		tuners[i] = tuner
	}
	return tuners
}

// GetCenterFrequency gets the center frequency
func (t b20xRXTuner) GetCenterFrequency() uint32 {
	freq := C.b20xDriver_GetRxFrequency(t.driver.handle, C.uint32_t(t.index))
	return uint32(freq)
}

// SetCenterFrequency sets the center frequency
func (t b20xRXTuner) SetCenterFrequency(freq uint32) error {
	fmt.Printf("b20xRXTuner.SetCenterFrequency() tuner id (%d) called with freq: %d\n", t.index, freq)
	C.b20xDriver_SetRxFrequency(t.driver.handle, C.uint32_t(t.index), C.uint32_t(freq))
	return nil
}

// GetUseableBandwidth gets the useable bandwidth
func (t b20xRXTuner) GetUseableBandwidth() uint32 {
	// TODO: Implement this
	return 0
}

// GetCurrentBandwidth gets the current bandwidth
func (t b20xRXTuner) GetCurrentBandwidth() uint32 {
	bandwidth := C.b20xDriver_GetRxBandwidth(t.driver.handle, C.uint32_t(t.index))
	return uint32(bandwidth)
}

// SetCurrentBandwidth sets the current bandwidth
func (t b20xRXTuner) SetCurrentBandwidth(bandwidth uint32) error {
	fmt.Printf("b20xRXTuner.SetCurrentBandwidth() called with bandwidth: %d\n", bandwidth)
	C.b20xDriver_SetRxBandwidth(t.driver.handle, C.uint32_t(t.index), C.uint32_t(bandwidth))
	return nil
}

// GetSampleRate gets the sample rate
func (t b20xRXTuner) GetSampleRate() uint32 {
	rate := C.b20xDriver_GetSampleRate(t.driver.handle, C.uint32_t(t.index))
	return uint32(rate)
}

// SetSampleRate sets the sample rate
func (t b20xRXTuner) SetSampleRate(rate uint32) error {
	fmt.Printf("b20xRXTuner.SetSampleRate() called with rate: %d\n", rate)
	C.b20xDriver_SetSampleRate(t.driver.handle, C.uint32_t(t.index), C.uint32_t(rate))
	return nil
}

// GetGain gets the gain
func (t b20xRXTuner) GetGain() uint32 {
	gain := C.b20xDriver_GetGain(t.driver.handle, C.uint32_t(t.index))
	return uint32(gain)
}

// SetGain sets the gain
func (t b20xRXTuner) SetGain(gain uint32) error {
	fmt.Printf("b20xRXTuner.SetGain() called with gain: %d\n", gain)
	C.b20xDriver_SetGain(t.driver.handle, C.uint32_t(t.index), C.uint32_t(gain))
	return nil
}

// GetDcBias gets the DC bias setting
func (t b20xRXTuner) GetDcBias() bool {
	dcBias := C.b20xDriver_GetDcBias(t.driver.handle, C.uint32_t(t.index))
	return bool(dcBias)
}

// SetDcBias sets the DC bias setting
func (t b20xRXTuner) SetDcBias(dcBias bool) error {
	fmt.Printf("b20xRXTuner.SetDcBias() called with dcBias: %t\n", dcBias)
	C.b20xDriver_SetDcBias(t.driver.handle, C.uint32_t(t.index), C.bool(dcBias))
	return nil
}

// GetAgc gets the AGC setting
func (t b20xRXTuner) GetAgc() bool {
	agc := C.b20xDriver_GetAgc(t.driver.handle, C.uint32_t(t.index))
	return bool(agc)
}

// SetAgc sets the AGC setting
func (t b20xRXTuner) SetAgc(agc bool) error {
	fmt.Printf("b20xRXTuner.SetAgc() called with agc: %t\n", agc)
	C.b20xDriver_SetAgc(t.driver.handle, C.uint32_t(t.index), C.bool(agc))
	return nil
}

// Start starts the tuner
func (t b20xRXTuner) Start() {
	fmt.Println("b20xRXTuner.Start() called")
	C.b20xDriver_StartRxStream(t.driver.handle, C.uint32_t(t.index))
}

// Stop stops the tuner
func (t b20xRXTuner) Stop() {
	fmt.Println("b20xRXTuner.Stop() called")
	C.b20xDriver_StopRxStream(t.driver.handle, C.uint32_t(t.index))
}

func (t b20xRXTuner) GetVisualizationData() driver.SpectralDataSlice {
	size := C.b20xDriver_GetPowerDbsSize(t.driver.handle, C.uint32_t(t.index))
	data := make([]float32, size)

	C.b20xDriver_GetPowerDbsData(t.driver.handle, C.uint32_t(t.index), (*C.float)(&data[0]), C.uint32_t(size))

	timestamp := time.Now().UnixNano()
	return driver.SpectralDataSlice{
		TimestampNs:  timestamp,
		ChannelIndex: int(t.index),
		CenterFreqHz: uint32(C.b20xDriver_GetRxFrequency(t.driver.handle, C.uint32_t(t.index))),
		SampleRateHz: uint32(C.b20xDriver_GetSampleRate(t.driver.handle, C.uint32_t(t.index))),
		FftSize:      int(size),
		PowerDb:      data,
		FreqMinHz:    float64(-C.b20xDriver_GetRxBandwidth(t.driver.handle, C.uint32_t(t.index)) / 2.0),
		FreqMaxHz:    float64(C.b20xDriver_GetRxBandwidth(t.driver.handle, C.uint32_t(t.index)) / 2.0),
	}
}

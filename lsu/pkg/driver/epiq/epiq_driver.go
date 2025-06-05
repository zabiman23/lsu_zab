package epiq

import (
	"fmt"
	"unsafe"

	"luf.co/lsu/pkg/driver"
)

// #cgo CFLAGS: -I.
// #cgo LDFLAGS: -L./build -lepiq_driver
// #include "cpp/epiq_driver.h"
// #include <stdlib.h>
// #include <stdint.h>
// #include <stdbool.h>
import "C"

// EpiqDriver is a driver for UHD based SDRs
type EpiqDriver struct {
	handle         C.EpiqDriverHandle
	tuners         []EpiqRXTuner
	coherentTuners []EpiqRXTuner
}

type EpiqRXTuner struct {
	driver *EpiqDriver
	index  uint32
}

// Init initializes the UHD driver
func (d *EpiqDriver) Init(args []string) error {
	fmt.Println("EpiqDriver.Init() called")

	cArgs := C.CString(args[0]) // Assuming args has at least one element.
	defer C.free(unsafe.Pointer(cArgs))

	d.handle = C.EpiqDriver_Init(cArgs)
	if d.handle == nil {
		return fmt.Errorf("EpiqDriver_Init failed")
	}

	numTuners := C.EpiqDriver_GetNumTuners(d.handle)
	d.tuners = make([]EpiqRXTuner, numTuners)
	for i := uint32(0); i < uint32(numTuners); i++ {
		d.tuners[i] = EpiqRXTuner{driver: d, index: i}
	}

	numCoherentTuners := C.EpiqDriver_GetNumCoherentTuners(d.handle)
	d.coherentTuners = make([]EpiqRXTuner, numCoherentTuners)
	for i := uint32(0); i < uint32(numCoherentTuners); i++ {
		d.coherentTuners[i] = EpiqRXTuner{driver: d, index: i}
	}

	return nil
}

// Close closes the UHD driver
func (d *EpiqDriver) Close() error {
	fmt.Println("EpiqDriver.Close() called")
	C.EpiqDriver_Close(d.handle)
	return nil
}

// GetNumTuners returns the number of tuners
func (d *EpiqDriver) GetNumTuners() uint32 {
	return uint32(len(d.tuners))
}

// GetNumCoherentTuners returns the number of coherent tuners
func (d *EpiqDriver) GetNumCoherentTuners() uint32 {
	return uint32(len(d.coherentTuners))
}

// GetTuningGranularity returns the tuning granularity
func (d *EpiqDriver) GetTuningGranularity() driver.TuningGranularity {
	// TODO: Implement this
	return driver.TuningGranularity{
		BoundaryHz: 1.0,
		Precision:  0.1,
	}
}

// GetStatus returns the status of the SDR
func (d *EpiqDriver) GetStatus() driver.SDRStatus {
	// TODO: Implement this
	return driver.SDRStatus{
		Alive: true,
		Position: driver.Location{
			Latitude:  float32(C.EpiqDriver_GetLatitude(d.handle)),
			Longitude: float32(C.EpiqDriver_GetLongitude(d.handle)),
			Elevation: float32(C.EpiqDriver_GetElevation(d.handle)),
		},
	}
}

// GetTuners returns the tuners
func (d *EpiqDriver) GetTuners() []driver.SDRTuner {
	tuners := make([]driver.SDRTuner, len(d.tuners))
	for i, tuner := range d.tuners {
		tuners[i] = tuner
	}
	return tuners
}

// GetCoherentTuners returns the coherent tuners
func (d *EpiqDriver) GetCoherentTuners() []driver.SDRCoherentTuner {
	tuners := make([]driver.SDRCoherentTuner, len(d.coherentTuners))
	for i, tuner := range d.coherentTuners {
		tuners[i] = tuner
	}
	return tuners
}

// GetCenterFrequency gets the center frequency
func (t EpiqRXTuner) GetCenterFrequency() uint32 {
	fmt.Println("EpiqRXTuner.GetCenterFrequency() called")
	freq := C.EpiqDriver_GetRxFrequency(t.driver.handle, C.uint32_t(t.index))
	return uint32(freq)
}

// SetCenterFrequency sets the center frequency
func (t EpiqRXTuner) SetCenterFrequency(freq uint32) error {
	fmt.Printf("EpiqRXTuner.SetCenterFrequency() called with freq: %d\n", freq)
	C.EpiqDriver_SetRxFrequency(t.driver.handle, C.uint32_t(t.index), C.uint32_t(freq))
	return nil
}

// GetUseableBandwidth gets the useable bandwidth
func (t EpiqRXTuner) GetUseableBandwidth() uint32 {
	// TODO: Implement this
	return 0
}

// GetCurrentBandwidth gets the current bandwidth
func (t EpiqRXTuner) GetCurrentBandwidth() uint32 {
	fmt.Println("EpiqRXTuner.GetCurrentBandwidth() called")
	bandwidth := C.EpiqDriver_GetRxBandwidth(t.driver.handle, C.uint32_t(t.index))
	return uint32(bandwidth)
}

// SetCurrentBandwidth sets the current bandwidth
func (t EpiqRXTuner) SetCurrentBandwidth(bandwidth uint32) error {
	fmt.Printf("EpiqRXTuner.SetCurrentBandwidth() called with bandwidth: %d\n", bandwidth)
	C.EpiqDriver_SetRxBandwidth(t.driver.handle, C.uint32_t(t.index), C.uint32_t(bandwidth))
	return nil
}

// GetSampleRate gets the sample rate
func (t EpiqRXTuner) GetSampleRate() uint32 {
	fmt.Println("EpiqRXTuner.GetSampleRate() called")
	rate := C.EpiqDriver_GetSampleRate(t.driver.handle, C.uint32_t(t.index))
	return uint32(rate)
}

// SetSampleRate sets the sample rate
func (t EpiqRXTuner) SetSampleRate(rate uint32) error {
	fmt.Printf("EpiqRXTuner.SetSampleRate() called with rate: %d\n", rate)
	C.EpiqDriver_SetSampleRate(t.driver.handle, C.uint32_t(t.index), C.uint32_t(rate))
	return nil
}

// GetGain gets the gain
func (t EpiqRXTuner) GetGain() uint32 {
	fmt.Println("EpiqRXTuner.GetGain() called")
	gain := C.EpiqDriver_GetGain(t.driver.handle, C.uint32_t(t.index))
	return uint32(gain)
}

// SetGain sets the gain
func (t EpiqRXTuner) SetGain(gain uint32) error {
	fmt.Printf("EpiqRXTuner.SetGain() called with gain: %d\n", gain)
	C.EpiqDriver_SetGain(t.driver.handle, C.uint32_t(t.index), C.uint32_t(gain))
	return nil
}

// GetDcBias gets the DC bias setting
func (t EpiqRXTuner) GetDcBias() bool {
	fmt.Println("EpiqRXTuner.GetDcBias() called")
	dcBias := C.EpiqDriver_GetDcBias(t.driver.handle, C.uint32_t(t.index))
	return bool(dcBias)
}

// SetDcBias sets the DC bias setting
func (t EpiqRXTuner) SetDcBias(dcBias bool) error {
	fmt.Printf("EpiqRXTuner.SetDcBias() called with dcBias: %t\n", dcBias)
	C.EpiqDriver_SetDcBias(t.driver.handle, C.uint32_t(t.index), C.bool(dcBias))
	return nil
}

// GetAgc gets the AGC setting
func (t EpiqRXTuner) GetAgc() bool {
	fmt.Println("EpiqRXTuner.GetAgc() called")
	agc := C.EpiqDriver_GetAgc(t.driver.handle, C.uint32_t(t.index))
	return bool(agc)
}

// SetAgc sets the AGC setting
func (t EpiqRXTuner) SetAgc(agc bool) error {
	fmt.Printf("EpiqRXTuner.SetAgc() called with agc: %t\n", agc)
	C.EpiqDriver_SetAgc(t.driver.handle, C.uint32_t(t.index), C.bool(agc))
	return nil
}

// Start starts the tuner
func (t EpiqRXTuner) Start() {
	fmt.Println("EpiqRXTuner.Start() called")
	C.EpiqDriver_StartRxStream(t.driver.handle, C.uint32_t(t.index))
}

// Stop stops the tuner
func (t EpiqRXTuner) Stop() {
	fmt.Println("EpiqRXTuner.Stop() called")
	C.EpiqDriver_StopRxStream(t.driver.handle, C.uint32_t(t.index))
}

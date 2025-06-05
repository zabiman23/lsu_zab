package uhd_holoscan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"luf.co/lsu/pkg/driver"
)

type UHDHoloscanDriver struct {
	tuners         []driver.SDRTuner
	coherentTuners []driver.SDRCoherentTuner
}

type UHDHoloscanRXTuner struct {
	driver          *UHDHoloscanDriver
	index           uint32
	centerFrequency uint32
	bandwidth       uint32
	sampleRate      uint32
	gain            uint32
	dcBias          bool
	agc             bool
	graphID         string
}

func (d *UHDHoloscanDriver) Init(args []string) error {
	fmt.Println("UHDHoloscanDriver.Init() called")

	// Initialize tuners (replace with actual Holoscan tuner count)
	numTuners := 2 // Example number of tuners
	d.tuners = make([]driver.SDRTuner, numTuners)
	for i := uint32(0); i < uint32(numTuners); i++ {
		holoscanRxTuner := &UHDHoloscanRXTuner{
			driver:          d,
			index:           i,
			centerFrequency: 0,
			bandwidth:       0,
			sampleRate:      0,
			gain:            0,
			dcBias:          false,
			agc:             false,
			graphID:         "",
		}
		d.tuners[i] = holoscanRxTuner
	}

	// Initialize coherent tuners (replace with actual Holoscan tuner count)
	numCoherentTuners := 1 // Example number of coherent tuners
	d.coherentTuners = make([]driver.SDRCoherentTuner, numCoherentTuners)
	for i := uint32(0); i < uint32(numCoherentTuners); i++ {
		holoscanRxTuner := &UHDHoloscanRXTuner{
			driver:          d,
			index:           i,
			centerFrequency: 0,
			bandwidth:       0,
			sampleRate:      0,
			gain:            0,
			dcBias:          false,
			agc:             false,
			graphID:         "",
		}
		d.coherentTuners[i] = holoscanRxTuner
	}

	return nil
}

func (d *UHDHoloscanDriver) Close() error {
	fmt.Println("UHDHoloscanDriver.Close() called")
	// TODO: Holoscan close here
	return nil
}

func (d *UHDHoloscanDriver) GetNumTuners() uint32 {
	return uint32(len(d.tuners))
}

func (d *UHDHoloscanDriver) GetNumCoherentTuners() uint32 {
	return uint32(len(d.coherentTuners))
}

func (d *UHDHoloscanDriver) GetTuningGranularity() driver.TuningGranularity {
	// TODO: Implement with Holoscan specifics
	return driver.TuningGranularity{
		BoundaryHz: 1.0,
		Precision:  0.1,
	}
}

func (d *UHDHoloscanDriver) GetStatus() driver.SDRStatus {
	// TODO: Implement with Holoscan specifics
	return driver.SDRStatus{
		Alive: true,
		Position: driver.Location{
			Latitude:  0.0, // Replace with actual Holoscan data
			Longitude: 0.0, // Replace with actual Holoscan data
			Elevation: 0.0, // Replace with actual Holoscan data
		},
	}
}

func (d *UHDHoloscanDriver) GetTuners() []driver.SDRTuner {
	return d.tuners
}

// GetCoherentTuners returns the coherent tuners
func (d *UHDHoloscanDriver) GetCoherentTuners() []driver.SDRCoherentTuner {
	return d.coherentTuners
}

// GetCenterFrequency gets the center frequency
func (t *UHDHoloscanRXTuner) GetCenterFrequency() uint32 {
	return t.centerFrequency
}

// SetCenterFrequency sets the center frequency
func (t *UHDHoloscanRXTuner) SetCenterFrequency(freq uint32) error {
	t.centerFrequency = freq
	return nil
}

// GetUseableBandwidth gets the useable bandwidth
func (t *UHDHoloscanRXTuner) GetUseableBandwidth() uint32 {
	// TODO: Implement with Holoscan specifics
	return 0
}

// GetCurrentBandwidth gets the current bandwidth
func (t *UHDHoloscanRXTuner) GetCurrentBandwidth() uint32 {
	return t.bandwidth
}

// SetCurrentBandwidth sets the current bandwidth
func (t *UHDHoloscanRXTuner) SetCurrentBandwidth(bandwidth uint32) error {
	t.bandwidth = bandwidth
	return nil
}

// GetSampleRate gets the sample rate
func (t *UHDHoloscanRXTuner) GetSampleRate() uint32 {
	return t.sampleRate
}

// SetSampleRate sets the sample rate
func (t *UHDHoloscanRXTuner) SetSampleRate(rate uint32) error {
	t.sampleRate = rate
	return nil
}

// GetGain gets the gain
func (t *UHDHoloscanRXTuner) GetGain() uint32 {
	return t.gain
}

// SetGain sets the gain
func (t *UHDHoloscanRXTuner) SetGain(gain uint32) error {
	t.gain = gain
	return nil
}

// GetDcBias gets the DC bias setting
func (t *UHDHoloscanRXTuner) GetDcBias() bool {
	return t.dcBias
}

// SetDcBias sets the DC bias setting
func (t *UHDHoloscanRXTuner) SetDcBias(dcBias bool) error {
	t.dcBias = dcBias
	return nil
}

// GetAgc gets the AGC setting
func (t UHDHoloscanRXTuner) GetAgc() bool {
	return t.agc
}

// SetAgc sets the AGC setting
func (t *UHDHoloscanRXTuner) SetAgc(agc bool) error {
	t.agc = agc
	return nil
}

// Start starts the tuner
func (t *UHDHoloscanRXTuner) Start() {
	fmt.Println("UHDHoloscanRXTuner.Start() called")
	fmt.Println("Tuner index:", t.index)
	fmt.Println("Center frequency:", t.centerFrequency)
	fmt.Println("Sample rate:", t.sampleRate)
	fmt.Println("Gain:", t.gain)
	fmt.Println("Bandwidth:", t.bandwidth)
	fmt.Println("DC Bias:", t.dcBias)
	fmt.Println("AGC:", t.agc)

	// Construct the payload as a Go map or struct
	payload := map[string]interface{}{
		"graph_config": map[string]interface{}{
			"modules": map[string]interface{}{
				"_usrp_ingest": map[string]interface{}{
					"operators": []map[string]interface{}{
						{
							"name": "PyUSRPIngest_4RtZGJ",
							"type": "PyUSRPIngest",
							"parameters": map[string]interface{}{
								"center_frequency": t.centerFrequency,
								"gain":             t.gain,
								"rate":             t.sampleRate,
								"batch_size":       8192,
								"antenna":          "RX2",
								"channels":         []uint32{t.index},
							},
						},
					},
				},
				"websocket_sink": map[string]interface{}{
					"operators": []map[string]interface{}{
						{
							"name": "FFTOp_KQ02nK",
							"type": "FFTOp",
							"parameters": map[string]interface{}{
								"nfft": 1024,
							},
						},
						{
							"name": "FFTWebSocketSink_UPVz-D",
							"type": "FFTWebSocketSink",
							"parameters": map[string]interface{}{
								"operator_label": "data_fft",
								"host":           "10.10.4.39", //This is hardcoded for the moment
								"port":           8080,
								"nfft":           1024,
								"update_rate_hz": 1000,
							},
						},
						{
							"name": "IQWebSocketSink_6kSBNj",
							"type": "IQWebSocketSink",
							"parameters": map[string]interface{}{
								"operator_label": "data_iq",
								"host":           "10.10.4.39", //This is hardcoded for the moment
								"port":           8080,
								"update_rate_hz": 1000,
								"output_len":     1024,
							},
						},
					},
				},
			},
			"graph": []map[string]interface{}{
				{
					"src": map[string]interface{}{
						"operator": "PyUSRPIngest_4RtZGJ",
					},
					"dst": map[string]interface{}{
						"operator": "IQWebSocketSink_6kSBNj",
					},
				},
				{
					"src": map[string]interface{}{
						"operator": "FFTOp_KQ02nK",
					},
					"dst": map[string]interface{}{
						"operator": "FFTWebSocketSink_UPVz-D",
					},
				},
				{
					"src": map[string]interface{}{
						"operator": "PyUSRPIngest_4RtZGJ",
					},
					"dst": map[string]interface{}{
						"operator": "FFTOp_KQ02nK",
					},
				},
			},
		},
		"active": true,
	}

	// Marshal the payload into JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("Error marshaling JSON:", err)
		return
	}

	// Create an HTTP client
	client := &http.Client{
		Timeout: time.Second * 10,
	}

	// Create the HTTP request (hardcoded for the moment)
	req, err := http.NewRequest("POST", "http://10.10.4.39:8081/graph", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}

	// Set the Content-Type header
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return
	}
	defer resp.Body.Close()

	// Check the response status code
	if resp.StatusCode != http.StatusOK {
		fmt.Println("Request failed with status code:", resp.StatusCode)
		// Read and print the response body for debugging
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		newStr := buf.String()
		fmt.Println(newStr)
		return
	}

	// Decode the JSON response body
	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		fmt.Println("Error decoding JSON response:", err)
		return
	}

	// Extract the "graph_id"
	graphID, ok := response["graph_id"].(string) // Assumes graph_id is a string
	if !ok {
		fmt.Println("graph_id not found or is not a string in the response")
		return
	}

	fmt.Println("Graph ID:", graphID)
	t.graphID = graphID
	fmt.Println("Request successful!")
}

// Stop stops the tuner
func (t *UHDHoloscanRXTuner) Stop() {
	fmt.Println("UHDHoloscanRXTuner.Stop() called")

	// Check if graphID is empty
	if t.graphID == "" {
		fmt.Println("No graph ID to stop.")
		return
	}

	// Create an HTTP client
	client := &http.Client{
		Timeout: time.Second * 20,
	}

	// Construct the DELETE request URL
	url := fmt.Sprintf("http://10.10.4.39:8081/graph/%s", t.graphID) // Hardcoded for the moment

	// Create the HTTP request
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		fmt.Println("Error creating DELETE request:", err)
		return
	}

	// Set only the Accept header to match curl
	req.Header.Set("Accept", "*/*")

	fmt.Println("DELETE Request URL:", url)
	fmt.Println("Headers:", req.Header)

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending DELETE request:", err)
		return
	}
	defer resp.Body.Close()

	// Check the response status code
	if resp.StatusCode != 204 {
		fmt.Println("DELETE request failed with status code:", resp.StatusCode)
		// Read and print the response body for debugging
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		newStr := buf.String()
		fmt.Println(newStr)
		return
	}

	fmt.Println("Graph stopped successfully!")
	t.graphID = "" // Reset graph ID after stopping
}

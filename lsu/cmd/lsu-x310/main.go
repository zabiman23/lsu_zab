package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"

	"luf.co/lsu/pkg/driver"
	"luf.co/lsu/pkg/driver/x310"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // dev‑only!
}

type SDRTunerData struct {
	CenterFrequency  uint32
	UseableBandwidth uint32
	CurrentBandwidth uint32
	SampleRate       uint32
	Gain             uint32
	DcBias           bool
	Agc              bool
	VizData          driver.SpectralDataSlice
}

type VisualizationData struct {
	Status            driver.SDRStatus
	TuningGranularity driver.TuningGranularity
	TunerData         []SDRTunerData
}

// Global channel SDR visualization data
var vizData chan VisualizationData

type signalMsg struct {
	Type      string                     `json:"type"` // "offer" | "answer" | "candidate"
	SDP       *webrtc.SessionDescription `json:"sdp,omitempty"`
	Candidate *webrtc.ICECandidateInit   `json:"candidate,omitempty"`
}

var (
	sensorID       string // Unique ID for this sensor
	driverName     string // Name of the SDR driver to use
	driverInitArgs string // Arguments string for driver initialization
	webrtcAPI      *webrtc.API
	sdrDev         driver.SDRDriver // Holds the initialized SDR driver
)

func main() {
	flag.StringVar(&sensorID, "id", "x310-001", "Unique ID for this sensor")
	flag.Parse()

	log.Printf("Starting sensor: ID=%s, Driver=%s", sensorID, driverName)

	// --- Initialize WebRTC API ---
	webrtcAPI = webrtc.NewAPI()

	// --- Initialize the web server
	server := &http.Server{
		Addr: ":8080",
	}

	// --- Initialize the SDR driver ---
	sdrDev = x310.NewX310Driver(sensorID)
	initArgs := []string{"type=x300"}
	if err := sdrDev.Init(initArgs); err != nil {
		log.Fatalf("Driver '%s' Init error: %v", driverName, err)
	}
	log.Println("Driver initialized successfully.")

	defer func() {
		log.Println("Closing SDR Driver...")
		if sdrDev != nil {
			if err := sdrDev.Close(); err != nil {
				log.Printf("Error closing driver '%s': %v", driverName, err)
			} else {
				log.Println("SDR Driver closed.")
			}
		}
	}()

	// --- Handle OS signals for graceful shutdown ---
	// Create a channel to receive OS signals.
	// Buffer size 1 is important so the notifier doesn't block.
	osSigChan := make(chan os.Signal, 1)
	signal.Notify(osSigChan, os.Interrupt, syscall.SIGTERM)

	// Create a 'done' channel to signal the worker goroutine to stop.
	done := make(chan bool)

	// Handle OS signals.
	go func() {
		// Block until a signal is received.
		sig := <-osSigChan
		log.Printf("\nReceived signal: %v, initiating shutdown...\n", sig)

		// Signal sdrLoop to stop
		done <- true
		// Wait for the sdrLoop to tell us it's done
		<-done
		close(done)
		log.Println("Shutting down webserver...")
		if err := server.Close(); err != nil {
			log.Fatalf("HTTP close error: %v", err)
		}
	}()

	// --- Initialize the global visualization data channel ---
	vizData = make(chan VisualizationData)
	go sdrLoop(vizData, done)

	http.HandleFunc("/signal", handleWS)
	http.HandleFunc("/command", handleCommand)

	log.Println("Listening on :8080")
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("HTTP server error: %v", err)
	}
}

func sdrLoop(vizData chan VisualizationData, done chan bool) {
	if done == nil {
		log.Println("Invalid state: 'done' channel is nil")
		return
	}
	defer func() {
		// Signal that the goroutine is done
		done <- true
	}()
	// Continuously read samples from the SDR and send them to the visualization channel.
	for {
		// Gather the viz data from the SDR device
		tuners := sdrDev.GetTuners()
		var tunerDataList []SDRTunerData
		for _, tuner := range tuners {
			// Create and append SDRTunerData for each tuner.
			tunerDataList = append(tunerDataList, SDRTunerData{
				CenterFrequency:  tuner.GetCenterFrequency(),
				UseableBandwidth: tuner.GetUseableBandwidth(),
				CurrentBandwidth: tuner.GetCurrentBandwidth(),
				SampleRate:       tuner.GetSampleRate(),
				Gain:             tuner.GetGain(),
				DcBias:           tuner.GetDcBias(),
				Agc:              tuner.GetAgc(),
				VizData:          tuner.GetVisualizationData(),
			})
		}
		vizDataToSend := VisualizationData{
			Status:            sdrDev.GetStatus(),
			TuningGranularity: sdrDev.GetTuningGranularity(),
			TunerData:         tunerDataList,
		}
		// To prevent blocking in the case that there is no receiver on the channel,
		// use the select statement to check if the channel is ready to send.
		select {
		case <-done:
			log.Println("Stopping SDR loop...")
			return
		case vizData <- vizDataToSend:
			// Successfully sent visualization data
		default:
			// No receiver on the channel, don't do anything
		}

		time.Sleep(16 * time.Millisecond) // 60 FPS
	}
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade:", err)
		return
	}
	defer ws.Close()

	peerConn, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		log.Println(err)
		return
	}
	defer peerConn.Close()

	peerConn.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		cand := c.ToJSON()
		msg := signalMsg{
			Type:      "candidate",
			Candidate: &cand,
		}
		if b, err := json.Marshal(msg); err == nil {
			ws.WriteMessage(websocket.TextMessage, b)
		}
	})

	peerConn.OnDataChannel(func(dc *webrtc.DataChannel) {
		log.Printf("Data channel %q opened\n", dc.Label())

		dc.OnOpen(dataChannelOpenHandler(dc))

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			log.Printf("rx <%s>: %s\n", dc.Label(), string(msg.Data))
		})
	})

	// Signalling loop:
	for {
		_, payload, err := ws.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			return
		}

		var m signalMsg
		if err = json.Unmarshal(payload, &m); err != nil {
			continue
		}

		switch m.Type {
		case "offer":
			if err = peerConn.SetRemoteDescription(*m.SDP); err != nil {
				log.Println(err)
				return
			}
			answer, _ := peerConn.CreateAnswer(nil)
			peerConn.SetLocalDescription(answer)

			b, _ := json.Marshal(signalMsg{Type: "answer", SDP: &answer})
			ws.WriteMessage(websocket.TextMessage, b)

		case "candidate":
			if m.Candidate != nil {
				peerConn.AddICECandidate(*m.Candidate)
			}
		}
	}
}

func dataChannelOpenHandler(dc *webrtc.DataChannel) func() {
	return func() {
		// Start a goroutine to send vizualization data continuously.
		go func() {
			for data := range vizData {
				// Convert the VisualizationData to JSON.
				b, err := json.Marshal(data)
				if err != nil {
					log.Println("Error marshaling visualization data:", err)
					return
				}
				// Send the JSON data over the Data Channel.
				if err := dc.SendText(string(b)); err != nil {
					log.Println("Failed sending visualization data:", err)
					return
				}
			}
		}()
	}
}

func handleCommand(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade:", err)
		return
	}
	defer ws.Close()

	// Read commands from the WebSocket connection
	for {
		_, payload, err := ws.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			return
		}

		var cmd map[string]interface{}
		if err = json.Unmarshal(payload, &cmd); err != nil {
			log.Println("Error unmarshaling command:", err)
			continue
		}

		// Process the command
		if err := processCommand(cmd); err != nil {
			log.Println("Error processing command:", err)
			continue
		}
	}
}

func processCommand(cmd map[string]interface{}) error {
	// Example command processing logic
	switch cmd["action"] {

	case "tuner_set_center_frequency":
		err := handleTunerSetCenterFrequency(cmd)
		if err != nil {
			fmt.Printf("Set tuner center frequency failed: %v\n", err)
		}

	case "tuner_set_current_bandwidth":
		err := handleTunerSetCurrentBandwidth(cmd)
		if err != nil {
			fmt.Printf("Set tuner current bandwidth failed: %v\n", err)
		}

	case "tuner_set_sample_rate":
		err := handleTunerSetSampleRate(cmd)
		if err != nil {
			fmt.Printf("Set tuner sample rate failed: %v\n", err)
		}

	case "tuner_set_gain":
		err := handleTunerSetGain(cmd)
		if err != nil {
			fmt.Printf("Set tuner gain failed: %v\n", err)
		}

	case "tuner_set_dc_bias":
		err := handleTunerSetDcBias(cmd)
		if err != nil {
			fmt.Printf("Set tuner DC bias failed: %v\n", err)
		}

	case "tuner_set_agc":
		err := handleTunerSetAgc(cmd)
		if err != nil {
			fmt.Printf("Set tuner AGC failed: %v\n", err)
		}

	default:
		log.Printf("Unknown command action: %s", cmd["action"])
	}
	return nil
}

func handleTunerSetCenterFrequency(cmd map[string]interface{}) error {
	if sdrDev == nil {
		return fmt.Errorf("SDR driver not initialized")
	}

	tunerIndex, ok := cmd["tuner_index"].(float64)
	if !ok {
		return fmt.Errorf("invalid tuner index")
	}
	freq, ok := cmd["frequency"].(float64)
	if !ok {
		return fmt.Errorf("invalid frequency value")
	}

	tuner := sdrDev.GetTuners()[uint32(tunerIndex)]
	if tuner == nil {
		return fmt.Errorf("tuner %d not found", tunerIndex)
	}

	return tuner.SetCenterFrequency(uint32(freq))
}

func handleTunerSetCurrentBandwidth(cmd map[string]interface{}) error {
	if sdrDev == nil {
		return fmt.Errorf("SDR driver not initialized")
	}

	tunerIndex, ok := cmd["tuner_index"].(float64)
	if !ok {
		return fmt.Errorf("invalid tuner index")
	}
	bandwidth, ok := cmd["bandwidth"].(float64)
	if !ok {
		return fmt.Errorf("invalid bandwidth value")
	}

	tuner := sdrDev.GetTuners()[uint32(tunerIndex)]
	if tuner == nil {
		return fmt.Errorf("tuner %d not found", tunerIndex)
	}

	return tuner.SetCurrentBandwidth(uint32(bandwidth))
}

func handleTunerSetSampleRate(cmd map[string]interface{}) error {
	if sdrDev == nil {
		return fmt.Errorf("SDR driver not initialized")
	}

	tunerIndex, ok := cmd["tuner_index"].(float64)
	if !ok {
		return fmt.Errorf("invalid tuner index")
	}
	rate, ok := cmd["sample_rate"].(float64)
	if !ok {
		return fmt.Errorf("invalid sample rate value")
	}

	tuner := sdrDev.GetTuners()[uint32(tunerIndex)]
	if tuner == nil {
		return fmt.Errorf("tuner %d not found", tunerIndex)
	}

	return tuner.SetSampleRate(uint32(rate))
}

func handleTunerSetGain(cmd map[string]interface{}) error {
	if sdrDev == nil {
		return fmt.Errorf("SDR driver not initialized")
	}

	tunerIndex, ok := cmd["tuner_index"].(float64)
	if !ok {
		return fmt.Errorf("invalid tuner index")
	}
	gain, ok := cmd["gain"].(float64)
	if !ok {
		return fmt.Errorf("invalid gain value")
	}

	tuner := sdrDev.GetTuners()[uint32(tunerIndex)]
	if tuner == nil {
		return fmt.Errorf("tuner %d not found", tunerIndex)
	}

	return tuner.SetGain(uint32(gain))
}

func handleTunerSetDcBias(cmd map[string]interface{}) error {
	if sdrDev == nil {
		return fmt.Errorf("SDR driver not initialized")
	}

	tunerIndex, ok := cmd["tuner_index"].(float64)
	if !ok {
		return fmt.Errorf("invalid tuner index")
	}
	dcBias, ok := cmd["dc_bias"].(bool)
	if !ok {
		return fmt.Errorf("invalid DC bias value")
	}

	tuner := sdrDev.GetTuners()[uint32(tunerIndex)]
	if tuner == nil {
		return fmt.Errorf("tuner %d not found", tunerIndex)
	}

	return tuner.SetDcBias(dcBias)
}

func handleTunerSetAgc(cmd map[string]interface{}) error {
	if sdrDev == nil {
		return fmt.Errorf("SDR driver not initialized")
	}

	tunerIndex, ok := cmd["tuner_index"].(float64)
	if !ok {
		return fmt.Errorf("invalid tuner index")
	}
	agc, ok := cmd["agc"].(bool)
	if !ok {
		return fmt.Errorf("invalid AGC value")
	}

	tuner := sdrDev.GetTuners()[uint32(tunerIndex)]
	if tuner == nil {
		return fmt.Errorf("tuner %d not found", tunerIndex)
	}

	return tuner.SetAgc(agc)
}

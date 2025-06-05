// sensor_signal.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"luf.co/lsu/pkg/common"
)

var (
	peerConnection *webrtc.PeerConnection
	dataChannel    *webrtc.DataChannel
	wsConn         *websocket.Conn
	sensorID       string
	serverAddr     string
	doneChan       chan struct{} // Channel to signal termination
)

// --- WebSocket Send Helper ---
func sendWsMessage(message common.SignalingMessage) error {
	if wsConn == nil {
		return fmt.Errorf("websocket connection is not established")
	}
	log.Printf("[SIGNALING] ---> Sending Type=%s to %s", message.Type, message.TargetID)
	return wsConn.WriteJSON(message)
}

// --- WebRTC Setup ---
func setupPeerConnection() error {
	config := webrtc.Configuration{ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}}}
	api := webrtc.NewAPI()
	var err error
	peerConnection, err = api.NewPeerConnection(config)
	if err != nil {
		return fmt.Errorf("failed to create peer connection: %w", err)
	}

	log.Println("[WEBRTC] PeerConnection created")

	// Handle local ICE candidates
	peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		log.Printf("[ICE] Generated Local Candidate")
		candidatePayload := common.CandidatePayload{Candidate: c.ToJSON()}
		msg := common.SignalingMessage{
			Type:     "candidate",
			SenderID: sensorID,
			TargetID: "source",         // Candidates go to the central source
			Payload:  candidatePayload, // Use the struct
		}
		if err := sendWsMessage(msg); err != nil {
			log.Printf("Error sending candidate: %v", err)
		}
	})

	// Handle connection state changes
	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("[WEBRTC] Peer Connection State: %s", state.String())
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateDisconnected {
			log.Println("PeerConnection closed or failed. Attempting cleanup/reconnect could happen here.")
			// Consider signaling main loop to try reconnecting
			// For now, just log it. Closing WS will trigger exit.
		}
	})

	// Create the data channel
	dataChannel, err = peerConnection.CreateDataChannel("sensor-data", nil)
	if err != nil {
		peerConnection.Close()
		return fmt.Errorf("failed create data channel: %w", err)
	}
	log.Println("[WEBRTC] DataChannel 'sensor-data' created")

	// Data Channel Handlers
	dataChannel.OnOpen(func() {
		log.Printf(">>> Data Channel '%s' OPENED!", dataChannel.Label())
		// Start sending data
		go func() {
			ticker := time.NewTicker(3 * time.Second) // Send slightly less often
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if dataChannel.ReadyState() != webrtc.DataChannelStateOpen {
						return
					} // Exit if channel closed
					dataToSend := common.SensorData{Timestamp: time.Now().UnixNano(), Frequency: 101.1e6, FFT: []float32{0.1, 0.2, 0.3}}
					payload, _ := json.Marshal(dataToSend)
					log.Printf("Sending sensor data: %s", string(payload))
					if err := dataChannel.Send(payload); err != nil {
						log.Printf("Error sending data: %v", err)
						if dataChannel.ReadyState() != webrtc.DataChannelStateOpen {
							return
						} // Re-check and exit if closed during send
					}
				case <-doneChan: // Listen for termination signal
					log.Println("Stopping data sending goroutine.")
					return
				}
			}
		}()
	})
	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		log.Printf("[COMMAND] Received command: %s", string(msg.Data))
		var cmd common.SensorCommand
		if err := json.Unmarshal(msg.Data, &cmd); err == nil {
			log.Printf("Executing command: %+v", cmd)
			// Add logic here to control SDR based on cmd
		} else {
			log.Printf("Error parsing command: %v", err)
		}
	})
	dataChannel.OnClose(func() { log.Printf("Data Channel '%s' CLOSED.", dataChannel.Label()) })
	dataChannel.OnError(func(err error) { log.Printf("Data Channel '%s' ERROR: %v", dataChannel.Label(), err) })

	// --- Start Offer Process ---
	log.Println("Creating Offer...")
	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		peerConnection.Close()
		return fmt.Errorf("failed create offer: %w", err)
	}

	if err := peerConnection.SetLocalDescription(offer); err != nil {
		peerConnection.Close()
		return fmt.Errorf("failed set local description: %w", err)
	}
	log.Println("Set Local Description (Offer)")

	// Send offer via WebSocket
	offerMsg := common.SignalingMessage{
		Type:     "offer",
		SenderID: sensorID,
		TargetID: "source",
		Payload:  offer.SDP,
	}
	return sendWsMessage(offerMsg)
}

// --- WebSocket Message Handling ---
func handleIncomingWsMessages() {
	defer func() {
		log.Println("WebSocket read loop terminated.")
		close(doneChan) // Signal termination to other goroutines
	}()
	for {
		messageType, messageData, err := wsConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			} else {
				log.Println("WebSocket connection closed by server.")
			}
			break
		}
		if messageType != websocket.TextMessage {
			continue
		}

		log.Printf("[SIGNALING] <--- Received raw: %s", string(messageData))

		var msg common.SignalingMessage
		if err := json.Unmarshal(messageData, &msg); err != nil {
			log.Printf("Error unmarshalling message: %v", err)
			continue
		}

		// Ignore messages not targeted to this sensor or from self
		if msg.TargetID != sensorID && msg.TargetID != "all" { // Allow broadcast later?
			log.Printf("Ignoring message targeted to %s", msg.TargetID)
			continue
		}

		switch msg.Type {
		case "answer":
			if peerConnection == nil {
				continue
			} // Ignore if PC not ready
			sdp, ok := msg.Payload.(string)
			if !ok {
				log.Println("Invalid answer payload")
				continue
			}
			log.Println("Received Answer")
			answer := webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: sdp}
			if err := peerConnection.SetRemoteDescription(answer); err != nil {
				log.Printf("Error setting remote description: %v", err)
			} else {
				log.Println("Set Remote Description (Answer)")
			}

		case "candidate":
			if peerConnection == nil {
				continue
			} // Ignore if PC not ready

			var candidatePayload common.CandidatePayload
			payloadBytes, _ := json.Marshal(msg.Payload)
			if err := json.Unmarshal(payloadBytes, &candidatePayload); err != nil {
				log.Printf("Error unmarshalling candidate payload: %v", err)
				continue
			}

			log.Printf("Received Remote Candidate: %+v", candidatePayload.Candidate)
			if err := peerConnection.AddICECandidate(candidatePayload.Candidate); err != nil {
				log.Printf("Error adding remote candidate: %v", err)
			}
		case "registered":
			log.Println("Successfully registered with signaling server.")
		case "error":
			log.Printf("Received error from signaling server: %v", msg.Payload)
			// Potentially trigger shutdown or retry depending on error

		default:
			log.Printf("Received unknown message type: %s", msg.Type)
		}
	}
}

// --- Main Function ---
func main() {
	// Command line flags
	flag.StringVar(&sensorID, "id", "sensor-default", "Unique ID for this sensor")
	flag.StringVar(&serverAddr, "server", "ws://localhost:8080/ws", "Address of the signaling server")
	flag.Parse()

	doneChan = make(chan struct{}) // Initialize termination channel

	log.Printf("Starting sensor: ID=%s, Server=%s", sensorID, serverAddr)

	// --- Connect to WebSocket Server ---
	var err error
	wsConn, _, err = websocket.DefaultDialer.Dial(serverAddr, nil)
	if err != nil {
		log.Fatalf("Failed to connect to signaling server %s: %v", serverAddr, err)
	}
	defer wsConn.Close()
	log.Println("Connected to signaling server.")

	// Send registration message
	registerMsg := common.SignalingMessage{Type: "register", SenderID: sensorID}
	if err := wsConn.WriteJSON(registerMsg); err != nil {
		log.Fatalf("Failed to send registration message: %v", err)
	}

	// Start WebSocket read loop in a goroutine
	go handleIncomingWsMessages()

	// --- Setup WebRTC ---
	if err := setupPeerConnection(); err != nil {
		log.Fatalf("Failed to setup WebRTC: %v", err)
	}

	// Handle graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	log.Println("Sensor running. Press Ctrl+C to exit.")

	select {
	case <-sigs:
		log.Println("Shutdown signal received.")
	case <-doneChan:
		log.Println("WebSocket connection lost or error encountered.")
	}

	// Cleanup
	log.Println("Cleaning up...")
	if peerConnection != nil && peerConnection.ConnectionState() != webrtc.PeerConnectionStateClosed {
		peerConnection.Close()
	}
	if wsConn != nil {
		// Optionally send a "disconnecting" message to the server here
		wsConn.Close() // Ensures read loop goroutine exits if not already
	}
	log.Println("Sensor stopped.")
}

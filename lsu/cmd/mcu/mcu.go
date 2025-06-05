package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings" // Added for string manipulation
	"sync"

	// For generating unique request IDs if needed internally
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"luf.co/lsu/pkg/common"
)

// --- Constants ---
const (
	RoleSensor                    = "sensor"
	RoleFrontend                  = "frontend"
	SourceID                      = "source"  // Identifier for messages originating from the MCU
	DataChannelSensorStreamPrefix = "stream-" // Prefix for data channels carrying sensor streams to frontends
)

// --- WebSocket Upgrader ---
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // Allow all origins
}

// --- Client Management ---
type Client struct {
	ID   string          // Unique ID (e.g., sensor-001, frontend-abc)
	Role string          // "sensor" or "frontend"
	Conn *websocket.Conn // WebSocket connection
}

// Maps to store clients, peer connections, and state
var (
	// WebSocket client tracking
	clients     = make(map[*websocket.Conn]*Client) // Map conn -> client info
	clientsByID = make(map[string]*Client)          // Map ID   -> client info
	// PeerConnection tracking
	sensorPeerConnections   = make(map[string]*webrtc.PeerConnection) // Map sensorID   -> sensor PC
	frontendPeerConnections = make(map[string]*webrtc.PeerConnection) // Map frontendID -> frontend PC
	// Data Channel tracking (for relaying)
	// Maps: frontendClientID -> sensorID -> *webrtc.DataChannel (the channel TO frontend FOR that sensor's stream)
	frontendDataChannels = make(map[string]map[string]*webrtc.DataChannel)
	// Request ID tracking (for command responses)
	// Maps: requestID -> frontendClientID (who made the request)
	requestIDMap = make(map[string]string)
	// Mutexes
	mapMutex    sync.RWMutex // Protects clients, clientsByID, peerConnections maps
	dcMapMutex  sync.RWMutex // Protects frontendDataChannels map
	reqMapMutex sync.RWMutex // Protects requestIDMap
)

// --- WebRTC API ---
var webrtcAPI *webrtc.API

// --- Signaling Logic ---

// sendToClient sends a JSON message to a specific client via WebSocket.
func sendToClient(targetID string, message common.SignalingMessage) error {
	mapMutex.RLock()
	client, ok := clientsByID[targetID]
	mapMutex.RUnlock()

	if !ok {
		// Log less verbosely if client disconnects race condition occurs
		// log.Printf("[SIGNALING] Error: Client %s not found for sending message type %s.", targetID, message.Type)
		return fmt.Errorf("client %s not found", targetID)
	}

	// Log selectively based on message type if needed (e.g., don't log every candidate)
	log.Printf("[SIGNALING] ---> Sending to %s: Type=%s", targetID, message.Type)
	// Use WriteJSON which handles marshalling and is safe for concurrent use.
	err := client.Conn.WriteJSON(message)
	if err != nil {
		log.Printf("[SIGNALING] Error writing JSON to client %s: %v", targetID, err)
		// Consider scheduling client cleanup if write fails
	}
	return err
}

// --- WebSocket Handler ---
// handleWebSocket manages incoming WebSocket connections and routes messages.
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()
	log.Println("WebSocket client connected:", conn.RemoteAddr())

	var currentClient *Client // Holds info after successful registration

	// Add connection temporarily (client is nil until registered)
	mapMutex.Lock()
	clients[conn] = nil
	mapMutex.Unlock()

	// --- Message Read Loop ---
	for {
		messageType, messageData, err := conn.ReadMessage()
		if err != nil {
			// Handle disconnection/errors
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error from %s: %v", conn.RemoteAddr(), err)
			} else {
				log.Println("WebSocket connection closed:", conn.RemoteAddr())
			}
			break // Exit loop
		}

		if messageType != websocket.TextMessage {
			continue
		} // Ignore non-text messages

		log.Printf("[SIGNALING] <--- Received raw from %s: %s", conn.RemoteAddr(), string(messageData))

		var msg common.SignalingMessage
		if err := json.Unmarshal(messageData, &msg); err != nil {
			log.Printf("Error unmarshalling message: %v", err)
			continue
		}

		// --- Process based on Type ---
		switch msg.Type {
		case "register":
			clientID := msg.SenderID
			clientRole := RoleFrontend // Default to frontend
			// Simple role detection based on ID prefix (adjust as needed)
			if strings.HasPrefix(clientID, "sensor-") {
				clientRole = RoleSensor
			}

			if clientID == "" {
				log.Println("Registration failed: Missing senderId")
				_ = conn.WriteJSON(common.SignalingMessage{Type: "error", Payload: "Registration failed: senderId required"})
				continue
			}

			mapMutex.Lock()
			if _, exists := clientsByID[clientID]; exists {
				log.Printf("Registration failed: ID %s (%s) already registered", clientID, clientRole)
				_ = conn.WriteJSON(common.SignalingMessage{Type: "error", Payload: fmt.Sprintf("ID %s already registered", clientID)})
				mapMutex.Unlock()
				// Consider disconnecting the old client? For now, reject new.
				continue
			}

			// Register new client
			currentClient = &Client{ID: clientID, Role: clientRole, Conn: conn}
			clients[conn] = currentClient
			clientsByID[clientID] = currentClient
			log.Printf("Client registered: ID=%s, Role=%s, Addr=%s", clientID, clientRole, conn.RemoteAddr())
			_ = conn.WriteJSON(common.SignalingMessage{Type: "registered", TargetID: clientID, SenderID: SourceID})

			// If it's a frontend, send the current sensor list
			if clientRole == RoleFrontend {
				var sensorList []string
				// Iterate safely over sensor PCs map
				for sensorID := range sensorPeerConnections {
					sensorList = append(sensorList, sensorID)
				}
				log.Printf("Sending sensor list to frontend %s: %v", clientID, sensorList)
				listMsg := common.SignalingMessage{
					Type:     "sensor_list",
					SenderID: SourceID,
					TargetID: clientID,
					Payload:  sensorList,
				}
				// Unlock mutex before sending message to avoid deadlock if sendToClient blocks
				mapMutex.Unlock()
				if err := sendToClient(clientID, listMsg); err != nil {
					log.Printf("Error sending sensor list to %s: %v", clientID, err)
				}
			} else {
				// If it's a sensor, maybe broadcast update to frontends?
				// (Implemented in cleanup for now)
				mapMutex.Unlock()
			}

		case "offer", "candidate":
			// Ensure client is registered
			if currentClient == nil {
				log.Println("Received WebRTC signal from unregistered client")
				_ = conn.WriteJSON(common.SignalingMessage{Type: "error", Payload: "Client not registered"})
				continue
			}
			// Route WebRTC signaling based on client role
			if currentClient.Role == RoleSensor {
				handleSensorWebRTCMessage(currentClient, msg)
			} else if currentClient.Role == RoleFrontend {
				handleFrontendWebRTCMessage(currentClient, msg)
			} else {
				log.Printf("Unknown role '%s' for client %s", currentClient.Role, currentClient.ID)
			}

		case "command": // Command from frontend intended for a sensor
			if currentClient == nil || currentClient.Role != RoleFrontend {
				log.Printf("Received 'command' from non-frontend or unregistered client %s", conn.RemoteAddr())
				continue
			}
			handleFrontendCommand(currentClient, msg)

		default:
			log.Printf("Received unknown message type '%s' from %s", msg.Type, conn.RemoteAddr())
		} // End switch msg.Type
	} // End message read loop

	// --- Cleanup on WebSocket Disconnect ---
	mapMutex.Lock()
	if currentClient != nil {
		clientID := currentClient.ID
		clientRole := currentClient.Role
		log.Printf("Client %s (%s) disconnected. Cleaning up...", clientID, clientRole)

		// Clean up associated WebRTC PeerConnection
		var pc *webrtc.PeerConnection
		var ok bool
		if clientRole == RoleSensor {
			pc, ok = sensorPeerConnections[clientID]
			if ok {
				delete(sensorPeerConnections, clientID)
			}
			// Also clean up any data channels associated with this sensor TO frontends
			dcMapMutex.Lock()
			for _, sensorChans := range frontendDataChannels {
				if dcToClose, dcOk := sensorChans[clientID]; dcOk {
					log.Printf("Closing relayed data channel stream-%s for frontend", clientID)
					_ = dcToClose.Close() // Best effort close
					delete(sensorChans, clientID)
				}
			}
			dcMapMutex.Unlock()
		} else { // RoleFrontend
			pc, ok = frontendPeerConnections[clientID]
			if ok {
				delete(frontendPeerConnections, clientID)
			}
			// Clean up data channel map entry for this frontend
			dcMapMutex.Lock()
			delete(frontendDataChannels, clientID)
			dcMapMutex.Unlock()
			// Clean up any pending request IDs from this frontend
			reqMapMutex.Lock()
			for reqID, feID := range requestIDMap {
				if feID == clientID {
					delete(requestIDMap, reqID)
				}
			}
			reqMapMutex.Unlock()
		}

		if ok && pc != nil {
			log.Printf("Closing PeerConnection for disconnected client %s", clientID)
			if err := pc.Close(); err != nil {
				log.Printf("Error closing PeerConnection for %s: %v", clientID, err)
			}
		}
		// Remove client from maps
		delete(clientsByID, clientID)

		// Broadcast updated sensor list if a sensor disconnected
		if clientRole == RoleSensor {
			var sensorList []string
			for sensorID := range sensorPeerConnections {
				sensorList = append(sensorList, sensorID)
			}
			log.Printf("Broadcasting updated sensor list due to %s disconnect: %v", clientID, sensorList)
			listMsg := common.SignalingMessage{Type: "sensor_list", SenderID: SourceID, Payload: sensorList}
			// Send to all remaining frontend clients
			for _, client := range clientsByID {
				if client.Role == RoleFrontend {
					listMsg.TargetID = client.ID
					// Unlock during send to avoid deadlock
					mapMutex.Unlock()
					_ = sendToClient(client.ID, listMsg)
					mapMutex.Lock() // Re-lock before next iteration/exit
				}
			}
		}

	} else {
		log.Println("Unregistered client disconnected.")
	}
	// Always remove the connection from the primary map
	delete(clients, conn)
	mapMutex.Unlock() // Release lock held since start of cleanup
}

// --- WebRTC Message Handlers ---

// handleSensorWebRTCMessage handles WebRTC signaling (offer/candidate) FROM sensors.
func handleSensorWebRTCMessage(sensor *Client, msg common.SignalingMessage) {
	sensorID := sensor.ID
	log.Printf("[WEBRTC Sensor] Processing '%s' from %s", msg.Type, sensorID)

	switch msg.Type {
	case "offer":
		sdp, ok := msg.Payload.(string)
		if !ok {
			log.Printf("[WEBRTC Sensor] Invalid offer payload from %s", sensorID)
			return
		}
		offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sdp}

		config := webrtc.Configuration{ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}}}
		peerConnection, err := webrtcAPI.NewPeerConnection(config)
		if err != nil {
			log.Printf("[WEBRTC Sensor] Failed create PC for %s: %v", sensorID, err)
			return
		}

		mapMutex.Lock()
		if oldPc, exists := sensorPeerConnections[sensorID]; exists {
			log.Printf("[WEBRTC Sensor] Closing existing PC for %s", sensorID)
			_ = oldPc.Close()
		}
		sensorPeerConnections[sensorID] = peerConnection
		mapMutex.Unlock()
		log.Printf("[WEBRTC Sensor] Created PC for %s", sensorID)

		cleanupOnError := func(errMsg string, err error) { /* ... (same cleanup logic as before, using sensorPeerConnections map) ... */
			log.Printf("[WEBRTC Sensor] [%s] %s: %v. Cleaning up.", sensorID, errMsg, err)
			_ = peerConnection.Close()
			mapMutex.Lock()
			if pc, ok := sensorPeerConnections[sensorID]; ok && pc == peerConnection {
				delete(sensorPeerConnections, sensorID)
			}
			mapMutex.Unlock()
			_ = sendToClient(sensorID, common.SignalingMessage{Type: "error", SenderID: SourceID, TargetID: sensorID, Payload: errMsg})
		}

		// Sensor PC Handlers
		peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) { /* ... (send candidate back to sensor) ... */
			if c == nil {
				return
			}
			log.Printf("[ICE] [%s] Sensor Local Candidate", sensorID)
			cP := common.CandidatePayload{Candidate: c.ToJSON()}
			sMsg := common.SignalingMessage{Type: "candidate", SenderID: SourceID, TargetID: sensorID, Payload: cP}
			if err := sendToClient(sensorID, sMsg); err != nil {
				log.Printf("[SIGNALING] Error sending candidate to sensor %s: %v", sensorID, err)
			}
		})
		peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) { /* ... (log state, handle cleanup on terminal states) ... */
			log.Printf("[WEBRTC Sensor] [%s] State: %s", sensorID, state.String())
			if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateDisconnected {
				log.Printf("[WEBRTC Sensor] [%s] Cleaning up terminal state PC", sensorID)
				mapMutex.Lock()
				if pc, ok := sensorPeerConnections[sensorID]; ok && pc == peerConnection {
					delete(sensorPeerConnections, sensorID)
				}
				mapMutex.Unlock()
			}
		})
		peerConnection.OnDataChannel(func(dc *webrtc.DataChannel) { // Data Channel FROM Sensor
			log.Printf("[WEBRTC Sensor] [%s] Data Channel Received: '%s' ID:%d", sensorID, dc.Label(), dc.ID())
			dc.OnOpen(func() { log.Printf("[WEBRTC Sensor] [%s] Data Channel '%s' OPENED.", sensorID, dc.Label()) })
			dc.OnClose(func() { log.Printf("[WEBRTC Sensor] [%s] Data Channel '%s' CLOSED.", sensorID, dc.Label()) })
			dc.OnError(func(err error) {
				log.Printf("[WEBRTC Sensor] [%s] Data Channel '%s' ERROR: %v", sensorID, dc.Label(), err)
			})
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				// --- Process messages received FROM the sensor ---
				log.Printf("[DATA SENSOR] [%s] Received on '%s': %d bytes", sensorID, dc.Label(), len(msg.Data))

				// Try parsing as WebRTCResponse first
				var resp common.WebRTCResponse
				errResp := json.Unmarshal(msg.Data, &resp)
				if errResp == nil && resp.Type == "response" {
					log.Printf("[RESPONSE SENSOR] [%s] Received: Cmd=%s, Success=%t, ReqID=%s", sensorID, resp.Command, resp.Success, resp.RequestID)
					// --- Relay Response back to Frontend via WebSocket ---
					reqMapMutex.RLock()
					targetFrontendID, ok := requestIDMap[resp.RequestID]
					reqMapMutex.RUnlock()
					if ok {
						responseMsg := common.SignalingMessage{
							Type:     "command_response",
							SenderID: SourceID,
							TargetID: targetFrontendID,
							Payload:  resp, // Forward the entire response object
						}
						if err := sendToClient(targetFrontendID, responseMsg); err != nil {
							log.Printf("[SIGNALING] Error relaying response (ReqID: %s) to frontend %s: %v", resp.RequestID, targetFrontendID, err)
						} else {
							log.Printf("[RELAY] Relayed response for ReqID %s to frontend %s", resp.RequestID, targetFrontendID)
						}
						// Clean up the request ID mapping
						reqMapMutex.Lock()
						delete(requestIDMap, resp.RequestID)
						reqMapMutex.Unlock()
					} else {
						log.Printf("[RESPONSE SENSOR] [%s] Warning: No frontend client found for RequestID: %s", sensorID, resp.RequestID)
					}
					return // Handled as response
				}

				// --- If not a response, treat as SensorData for relaying ---
				// (Assuming FFT data or similar stream on the default channel)
				log.Printf("[RELAY] Relaying data from %s (%d bytes)", sensorID, len(msg.Data))
				dcMapMutex.RLock() // Lock for reading frontend DC map
				// Iterate over connected frontend clients
				for feClientID, sensorChans := range frontendDataChannels {
					// Find the specific channel for this sensor's stream TO this frontend
					targetDc, dcExists := sensorChans[sensorID]
					if dcExists && targetDc.ReadyState() == webrtc.DataChannelStateOpen {
						log.Printf("[RELAY] Forwarding %d bytes from %s to %s", len(msg.Data), sensorID, feClientID)
						err := targetDc.Send(msg.Data) // Forward raw bytes
						if err != nil {
							log.Printf("[RELAY] Error relaying data to frontend %s for sensor %s: %v", feClientID, sensorID, err)
						}
					}
					// else { log.Printf("[RELAY] Skipping relay to %s for %s (DC not ready/found)", feClientID, sensorID) } // Too verbose?
				}
				dcMapMutex.RUnlock()

			}) // End Sensor DC OnMessage
		}) // End Sensor OnDataChannel

		// Process Offer/Answer
		if err := peerConnection.SetRemoteDescription(offer); err != nil {
			cleanupOnError("Sensor SetRemoteDesc(offer) failed", err)
			return
		}
		answer, err := peerConnection.CreateAnswer(nil)
		if err != nil {
			cleanupOnError("Sensor CreateAnswer failed", err)
			return
		}
		if err := peerConnection.SetLocalDescription(answer); err != nil {
			cleanupOnError("Sensor SetLocalDesc(answer) failed", err)
			return
		}
		log.Printf("[WEBRTC Sensor] Completed Offer/Answer for %s", sensorID)
		answerMsg := common.SignalingMessage{Type: "answer", SenderID: SourceID, TargetID: sensorID, Payload: answer.SDP}
		if err := sendToClient(sensorID, answerMsg); err != nil {
			log.Printf("[SIGNALING] Error sending answer to sensor %s: %v", sensorID, err)
		}

	case "candidate": // Candidate FROM Sensor
		mapMutex.RLock()
		peerConnection, pcExists := sensorPeerConnections[sensorID]
		mapMutex.RUnlock()
		if !pcExists {
			log.Printf("[WEBRTC Sensor] Received candidate for unknown session %s", sensorID)
			return
		}

		var cP common.CandidatePayload
		pBytes, _ := json.Marshal(msg.Payload)
		if err := json.Unmarshal(pBytes, &cP); err != nil {
			log.Printf("[WEBRTC Sensor] Error unmarshalling candidate from %s: %v", sensorID, err)
			return
		}
		if err := peerConnection.AddICECandidate(cP.Candidate); err != nil {
			log.Printf("[WEBRTC Sensor] Error adding remote candidate from %s: %v", sensorID, err)
		}

	default:
		log.Printf("Unknown Sensor WebRTC message type '%s'", msg.Type)
	}
}

// handleFrontendWebRTCMessage handles WebRTC signaling (offer/candidate) FROM frontend clients.
func handleFrontendWebRTCMessage(frontend *Client, msg common.SignalingMessage) {
	frontendID := frontend.ID
	log.Printf("[WEBRTC Frontend] Processing '%s' from %s", msg.Type, frontendID)

	switch msg.Type {
	case "offer": // Offer FROM Frontend
		sdp, ok := msg.Payload.(string)
		if !ok {
			log.Printf("[WEBRTC Frontend] Invalid offer payload from %s", frontendID)
			return
		}
		offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sdp}

		config := webrtc.Configuration{ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}}}
		peerConnection, err := webrtcAPI.NewPeerConnection(config)
		if err != nil {
			log.Printf("[WEBRTC Frontend] Failed create PC for %s: %v", frontendID, err)
			return
		}

		mapMutex.Lock()
		if oldPc, exists := frontendPeerConnections[frontendID]; exists {
			log.Printf("[WEBRTC Frontend] Closing existing PC for %s", frontendID)
			_ = oldPc.Close()
		}
		frontendPeerConnections[frontendID] = peerConnection
		// Initialize data channel map for this frontend
		dcMapMutex.Lock()
		frontendDataChannels[frontendID] = make(map[string]*webrtc.DataChannel)
		dcMapMutex.Unlock()
		mapMutex.Unlock()
		log.Printf("[WEBRTC Frontend] Created PC for %s", frontendID)

		cleanupOnError := func(errMsg string, err error) { /* ... (similar cleanup, using frontendPeerConnections map and frontendDataChannels map) ... */
			log.Printf("[WEBRTC Frontend] [%s] %s: %v. Cleaning up.", frontendID, errMsg, err)
			_ = peerConnection.Close()
			mapMutex.Lock()
			if pc, ok := frontendPeerConnections[frontendID]; ok && pc == peerConnection {
				delete(frontendPeerConnections, frontendID)
			}
			mapMutex.Unlock()
			dcMapMutex.Lock()
			delete(frontendDataChannels, frontendID)
			dcMapMutex.Unlock()
			_ = sendToClient(frontendID, common.SignalingMessage{Type: "error", SenderID: SourceID, TargetID: frontendID, Payload: errMsg})
		}

		// Frontend PC Handlers
		peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) { /* ... (send candidate back to frontend) ... */
			if c == nil {
				return
			}
			log.Printf("[ICE] [%s] Frontend Local Candidate", frontendID)
			cP := common.CandidatePayload{Candidate: c.ToJSON()}
			sMsg := common.SignalingMessage{Type: "candidate", SenderID: SourceID, TargetID: frontendID, Payload: cP}
			if err := sendToClient(frontendID, sMsg); err != nil {
				log.Printf("[SIGNALING] Error sending candidate to frontend %s: %v", frontendID, err)
			}
		})
		peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) { /* ... (log state, handle cleanup) ... */
			log.Printf("[WEBRTC Frontend] [%s] State: %s", frontendID, state.String())
			if state == webrtc.PeerConnectionStateConnected {
				// --- Create Data Channels TO Frontend for Sensor Streams ---
				log.Printf("[WEBRTC Frontend] [%s] Connected! Attempting to create relay data channels...", frontendID)
				mapMutex.RLock()  // Need read lock for sensorPeerConnections
				dcMapMutex.Lock() // Need write lock for frontendDataChannels
				if _, exists := frontendDataChannels[frontendID]; !exists {
					frontendDataChannels[frontendID] = make(map[string]*webrtc.DataChannel) // Ensure map exists
				}

				for sensorID := range sensorPeerConnections {
					// Check if channel already exists (e.g., from previous attempt)
					if _, dcExists := frontendDataChannels[frontendID][sensorID]; !dcExists {
						label := DataChannelSensorStreamPrefix + sensorID
						log.Printf("[WEBRTC Frontend] [%s] Creating data channel '%s'", frontendID, label)
						// Create channel TO the frontend for this sensor stream
						// We use Ordered=false, MaxRetransmits=0 for potentially lossy real-time streams
						// Adjust if reliable/ordered delivery is needed for FFT
						ordered := false
						maxRetransmits := uint16(0)
						options := &webrtc.DataChannelInit{
							Ordered:        &ordered,
							MaxRetransmits: &maxRetransmits,
						}
						dataChannelToFrontend, err := peerConnection.CreateDataChannel(label, options)
						if err != nil {
							log.Printf("[WEBRTC Frontend] [%s] Failed to create data channel '%s': %v", frontendID, label, err)
						} else {
							// Store the created channel
							frontendDataChannels[frontendID][sensorID] = dataChannelToFrontend
							// Set up handlers for this specific outgoing channel (optional)
							dataChannelToFrontend.OnOpen(func() { log.Printf("[RELAY DC] [%s] Data channel '%s' to frontend opened.", frontendID, label) })
							dataChannelToFrontend.OnError(func(err error) { log.Printf("[RELAY DC] [%s] Data channel '%s' error: %v", frontendID, label, err) })
							// No OnMessage needed here as MCU only sends on these
						}
					}
				}
				dcMapMutex.Unlock()
				mapMutex.RUnlock()

			} else if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateDisconnected {
				log.Printf("[WEBRTC Frontend] [%s] Cleaning up terminal state PC", frontendID)
				mapMutex.Lock()
				if pc, ok := frontendPeerConnections[frontendID]; ok && pc == peerConnection {
					delete(frontendPeerConnections, frontendID)
				}
				mapMutex.Unlock()
				dcMapMutex.Lock()
				delete(frontendDataChannels, frontendID)
				dcMapMutex.Unlock()
			}
		})
		// No OnDataChannel needed here if MCU initiates all relay channels

		// Process Offer/Answer
		if err := peerConnection.SetRemoteDescription(offer); err != nil {
			cleanupOnError("Frontend SetRemoteDesc(offer) failed", err)
			return
		}
		answer, err := peerConnection.CreateAnswer(nil)
		if err != nil {
			cleanupOnError("Frontend CreateAnswer failed", err)
			return
		}
		if err := peerConnection.SetLocalDescription(answer); err != nil {
			cleanupOnError("Frontend SetLocalDesc(answer) failed", err)
			return
		}
		log.Printf("[WEBRTC Frontend] Completed Offer/Answer for %s", frontendID)
		answerMsg := common.SignalingMessage{Type: "answer", SenderID: SourceID, TargetID: frontendID, Payload: answer.SDP}
		if err := sendToClient(frontendID, answerMsg); err != nil {
			log.Printf("[SIGNALING] Error sending answer to frontend %s: %v", frontendID, err)
		}

	case "candidate": // Candidate FROM Frontend
		mapMutex.RLock()
		peerConnection, pcExists := frontendPeerConnections[frontendID]
		mapMutex.RUnlock()
		if !pcExists {
			log.Printf("[WEBRTC Frontend] Received candidate for unknown session %s", frontendID)
			return
		}

		var cP common.CandidatePayload
		pBytes, _ := json.Marshal(msg.Payload)
		if err := json.Unmarshal(pBytes, &cP); err != nil {
			log.Printf("[WEBRTC Frontend] Error unmarshalling candidate from %s: %v", frontendID, err)
			return
		}
		if err := peerConnection.AddICECandidate(cP.Candidate); err != nil {
			log.Printf("[WEBRTC Frontend] Error adding remote candidate from %s: %v", frontendID, err)
		}

	default:
		log.Printf("Unknown Frontend WebRTC message type '%s'", msg.Type)
	}
}

// handleFrontendCommand processes commands sent from frontend clients via WebSocket.
func handleFrontendCommand(frontend *Client, msg common.SignalingMessage) {
	frontendID := frontend.ID
	targetSensorID := msg.TargetID // Command message's target is the sensor

	log.Printf("[COMMAND] Received command from frontend %s for sensor %s", frontendID, targetSensorID)

	// Find the target sensor's PeerConnection
	mapMutex.RLock()
	// sensorPc, pcExists := sensorPeerConnections[targetSensorID]
	// mapMutex.RUnlock()
	// if !pcExists {
	// 	log.Printf("[COMMAND] Target sensor %s not found or not connected.", targetSensorID)
	// 	errMsg := common.SignalingMessage{Type: "error", SenderID: SourceID, TargetID: frontendID, Payload: fmt.Sprintf("Target sensor %s not connected", targetSensorID)}
	// 	_ = sendToClient(frontendID, errMsg)
	// 	return
	// }

	// Find the Data Channel for the sensor (assume label "sensor-data")
	// Note: This assumes the sensor created the channel with this label.
	// A more robust way might involve storing the DC reference when sensor connects.
	var sensorDc *webrtc.DataChannel

	if sensorDc == nil || sensorDc.ReadyState() != webrtc.DataChannelStateOpen {
		log.Printf("[COMMAND] Data channel 'sensor-data' for target sensor %s not found or not open.", targetSensorID)
		errMsg := common.SignalingMessage{Type: "error", SenderID: SourceID, TargetID: frontendID, Payload: fmt.Sprintf("Data channel to sensor %s not ready", targetSensorID)}
		_ = sendToClient(frontendID, errMsg)
		return
	}

	// Marshal the command payload (which should be WebRTCCommand struct)
	commandPayloadBytes, err := json.Marshal(msg.Payload)
	if err != nil {
		log.Printf("[COMMAND] Failed to marshal command payload for relay: %v", err)
		errMsg := common.SignalingMessage{Type: "error", SenderID: SourceID, TargetID: frontendID, Payload: "Internal server error: Cannot marshal command"}
		_ = sendToClient(frontendID, errMsg)
		return
	}

	// Extract request ID if present in the payload for mapping
	var webRTCCommand common.WebRTCCommand
	if err := json.Unmarshal(commandPayloadBytes, &webRTCCommand); err == nil && webRTCCommand.RequestID != "" {
		reqMapMutex.Lock()
		requestIDMap[webRTCCommand.RequestID] = frontendID
		reqMapMutex.Unlock()
		log.Printf("[COMMAND] Stored request ID %s for frontend %s", webRTCCommand.RequestID, frontendID)
	} else if err != nil {
		log.Printf("[COMMAND] Warning: Could not unmarshal relayed command payload to extract request ID: %v", err)
	}

	// Relay the command payload over the sensor's data channel
	log.Printf("[RELAY] Relaying command to sensor %s (%d bytes)", targetSensorID, len(commandPayloadBytes))
	if err := sensorDc.Send(commandPayloadBytes); err != nil {
		log.Printf("[RELAY] Error relaying command to sensor %s: %v", targetSensorID, err)
		// Clean up request ID map if send fails?
		if webRTCCommand.RequestID != "" {
			reqMapMutex.Lock()
			delete(requestIDMap, webRTCCommand.RequestID)
			reqMapMutex.Unlock()
		}
		errMsg := common.SignalingMessage{Type: "error", SenderID: SourceID, TargetID: frontendID, Payload: fmt.Sprintf("Failed to send command to sensor %s", targetSensorID)}
		_ = sendToClient(frontendID, errMsg)
		return
	}
}

// --- Main Function ---
func main() {
	listenAddr := flag.String("addr", ":8080", "Address:Port to listen on")
	flag.Parse()

	// Initialize WebRTC API
	webrtcAPI = webrtc.NewAPI()

	// Setup HTTP handler for WebSocket
	http.HandleFunc("/ws", handleWebSocket)

	// Start server
	log.Printf("MCU (Signaling + WebRTC Source) server listening on %s", *listenAddr)
	if err := http.ListenAndServe(*listenAddr, nil); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
} // End main

// common.go
package common

import "github.com/pion/webrtc/v4"

// SignalingMessage definition remains the same...
type SignalingMessage struct {
	Type     string      `json:"type"`
	SenderID string      `json:"senderId"`
	TargetID string      `json:"targetId"`
	Payload  interface{} `json:"payload"`
}
type CandidatePayload struct {
	Candidate webrtc.ICECandidateInit `json:"candidate"`
}

// --- UPDATED: Command structure for WebRTC Data Channel ---
// Used for messages sent TO the sensor over the Data Channel
type WebRTCCommand struct {
	Command string        `json:"command"` // e.g., "tuner_set_center_frequency"
	Args    []interface{} `json:"args"`    // Flexible arguments (e.g., [0, 101100000])
	// Optional: Add a unique request ID for tracking responses
	RequestID string `json:"requestId,omitempty"`
}

// --- NEW: Response structure for WebRTC Data Channel ---
// Used for messages sent FROM the sensor back to the mcu
type WebRTCResponse struct {
	Type      string      `json:"type"`                // "response"
	RequestID string      `json:"requestId,omitempty"` // Correlates to the command
	Command   string      `json:"command"`             // Original command name
	Success   bool        `json:"success"`
	Error     string      `json:"error,omitempty"`  // Error message if success is false
	Result    interface{} `json:"result,omitempty"` // Result payload if success is true (e.g., gain value, status struct)
}

// Example structures for sensor data (FFT stream) - remains the same
type SensorData struct {
	Timestamp int64     `json:"timestamp"`
	Frequency float64   `json:"frequency"`
	FFT       []float32 `json:"fft_data"`
}

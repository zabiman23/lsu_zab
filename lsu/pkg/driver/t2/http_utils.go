package t2

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

func ParseHTTPResponse(resp *http.Response, target any) error {

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %w", err)
	}

	defer resp.Body.Close()

	// Check the Content-Type header
	// The response to collector config POST returns a plain text (eg: /ddc/v1/collector/<collector id>)
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		logger.Debug("Server returned non-JSON response:", slog.String("body", string(body)))
		return nil
	}

	// Handle different status code ranges
	// Ref: https://go.dev/src/net/http/status.go
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		// Success: Unmarshal the JSON into the target
		if target != nil {
			if err := json.Unmarshal(body, target); err != nil {
				return fmt.Errorf("error unmarshalling JSON: %w", err)
			}
		}
		return nil

	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		// Redirection: Handle as needed (e.g., log or return an error)
		return fmt.Errorf("redirection status code: %d, body: %s", resp.StatusCode, string(body))

	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		// Client error: Return a specific error
		return fmt.Errorf("client error status code: %d, body: %s", resp.StatusCode, string(body))

	case resp.StatusCode >= 500:
		// Server error: Return a specific error
		return fmt.Errorf("server error status code: %d, body: %s", resp.StatusCode, string(body))

	default:
		// Unexpected status code
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}
}

// http_client.go - HTTP client helpers for HTL-T2 driver
package t2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

func GetCoreInfo(url string) (CoreInfoResponse, error) {
	resp, err := http.Get(url)
	if err != nil {
		logger.Error("Error fetching core info", slog.String("error", err.Error()))
		return CoreInfoResponse{}, err
	}
	defer resp.Body.Close()

	var data CoreInfoResponse
	if err := ParseHTTPResponse(resp, &data); err != nil {
		logger.Error("Error parsing core info response", slog.String("error", err.Error()))
		return CoreInfoResponse{}, err
	}
	return data, nil
}

func GetDdcCapabilities(url string) (DdcCapabilitiesResponse, error) {
	resp, err := http.Get(url)
	if err != nil {
		logger.Error("Error fetching Ddc capabilities", slog.String("error", err.Error()))
		return DdcCapabilitiesResponse{}, err
	}
	defer resp.Body.Close()

	var data DdcCapabilitiesResponse
	if err := ParseHTTPResponse(resp, &data); err != nil {
		logger.Error("Error parsing ddc capabilities response", slog.String("error", err.Error()))
		return DdcCapabilitiesResponse{}, err
	}
	return data, nil
}

func PostCollectorConfig(url string, config DdcCollector) error {
	// Convert the config struct to JSON
	jsonData, err := json.Marshal(config)
	if err != nil {
		logger.Error("Error marshalling config to JSON", slog.String("error", err.Error()))
		return err
	}

	// Create a new HTTP POST request
	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		logger.Error("Error creating POST request", slog.String("error", err.Error()))
		return err
	}
	defer resp.Body.Close()

	// Add debug logs to trace the JSON payload, request headers, and response status and body
	logger.Debug("Posting to URL: ", slog.String("url", url))
	logger.Debug("JSON Payload: ", slog.String("payload", string(jsonData)))
	logger.Debug("Request Headers: ", slog.Any("headers", resp.Header))

	var cfgStatus DdcCollector
	if err := ParseHTTPResponse(resp, &cfgStatus); err != nil {
		logger.Error("Error parsing ddc collector config response", slog.String("error", err.Error()))
		return err
	}
	logger.Debug("Collector configuration posted successfully.")
	return nil
}

func GetCollectorStatus(url string) DdcCollector {
	resp, err := http.Get(url)
	if err != nil {
		logger.Error("Error fetching collector status:", slog.String(("error"), err.Error()))
		return DdcCollector{}
	}
	defer resp.Body.Close()

	var status DdcCollector
	if err := ParseHTTPResponse(resp, &status); err != nil {
		logger.Error("Error parsing ddc collector status response:", slog.String(("error"), err.Error()))
		return DdcCollector{}
	}
	return status
}

func DeleteCollectorConfig(url string, cstate DdcCollector, id uint32) bool {

	if cstate.UniqueID == "" {
		logger.Error("Error: unique_id not found for collector")
		return false
	}

	// Prepare the JSON body
	bodyData := map[string]any{
		"collector_id": id,
		"unique_id":    cstate.UniqueID,
	}
	jsonBody, err := json.Marshal(bodyData)
	logger.Debug(fmt.Sprintf("DBG - Sending JSON body: %s\n", string(jsonBody)))
	if err != nil {
		logger.Error("Error marshalling JSON body:", slog.String(("error"), err.Error()))
		return false
	}

	// Create a new HTTP DELETE request with body. There is no support for http.Delete()
	// funcion in the Go standard library.
	req, err := http.NewRequest("DELETE", url, bytes.NewReader(jsonBody))
	if err != nil {
		logger.Error("Error creating DELETE request:", slog.String(("error"), err.Error()))
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	// Debug log for request headers and body
	logger.Debug(fmt.Sprintf("DBG - Request URL: %s\n", req.URL))
	logger.Debug(fmt.Sprintf("DBG - Request Headers: %+v\n", req.Header))
	logger.Debug(fmt.Sprintf("DBG - Request Body: %s\n", string(jsonBody)))

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Error sending DELETE request:", slog.String(("error"), err.Error()))
		return false
	}
	defer resp.Body.Close()

	// Check the response status code
	if resp.StatusCode == 204 {
		logger.Info("Collector configuration deleted successfully.")
		return true
	} else if resp.StatusCode == 404 {
		logger.Error(fmt.Sprintf("Error: Collector with ID %d not found.\n", id))
	} else {
		logger.Info(fmt.Sprintf("Failed to delete collector configuration. Status code: %d\n", resp.StatusCode))
	}
	return false
}

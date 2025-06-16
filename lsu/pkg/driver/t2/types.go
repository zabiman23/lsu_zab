// types.go - All type definitions for the HTL-T2 driver
package t2

import "encoding/json"

// CoreInfoResponse represents some of the response from the /core/info/v1 endpoint
type CoreInfoResponse struct {
	UnitInfo struct {
		Name            string `json:"name"`
		SerialNumber    int    `json:"serial_number"`
		SoftwareVersion string `json:"software_version"`
	} `json:"unit_info"`

	UnitStatus struct {
		TimePositionStatus struct {
			PositionStatus struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
				Altitude  float64 `json:"altitude"`
			} `json:"position_status"`
		} `json:"time_position_status"`
	} `json:"unit_status"`

	TunerTraits []struct {
		ID       int `json:"id"`
		Segments []struct {
			BandwidthHz    int64 `json:"bandwidth_hz"`
			CoveredRangeHz struct {
				StartHz int64 `json:"start_hz"`
				StopHz  int64 `json:"stop_hz"`
			} `json:"covered_range_hz"`
		} `json:"segments"`
	} `json:"tuner_traits"`

	CoherencyGroups []struct {
		GroupID  int   `json:"group_id"`
		TunerIDs []int `json:"tuner_ids"`
	} `json:"coherency_groups"`
}

type DdcCapabilitiesResponse struct {
	NumTuners       int `json:"num_tuners"`
	CoherencyGroups []struct {
		GroupID  int   `json:"group_id"`
		TunerIDs []int `json:"tuner_ids"`
	} `json:"coherency_groups"`

	TunerTraits []struct {
		ID              int `json:"id"`
		TunableSegments []struct {
			CoveredRangeStartHz int64 `json:"covered_range_start_hz"`
			CoveredRangeStopHz  int64 `json:"covered_range_stop_hz"`
			TunableRangeStartHz int64 `json:"tune_range_start_hz"`
			TunableRangeStopHz  int64 `json:"tune_range_stop_hz"`
			SampleRateHz        int64 `json:"sample_rate_hz"`
			IFBandwidthHz       int64 `json:"if_bandwidth_hz"`
		} `json:"tunable_segments"`
	} `json:"tuner_traits"`
}

type DdcCollector struct {
	AGCSettings struct {
		Attenuation_dB float64 `json:"attenuation_dB"`
		Enabled        bool    `json:"enable"`
	} `json:"agc_settings"`
	BandwidthHz       uint64 `json:"bandwidth_hz"`
	CenterFreqHz      uint64 `json:"center_frequency_hz"`
	CollectorID       uint32 `json:"collector_id"`
	EnablePreamp      bool   `json:"enable_preamp"`
	FrequencyOffsetHz uint64 `json:"frequency_offset_hz"`
	IQStream          struct {
		DucBufferSizeMb uint32 `json:"duc_buffer_size_mb"`
		Encoding        int32  `json:"encoding"`
		HostName        string `json:"host_name"`
		LocalPort       uint32 `json:"local_port"`
		MacAddress      string `json:"mac_address"`
		PayloadSize     uint32 `json:"payload_size"`
		Port            uint32 `json:"port"`
		SfpID           uint32 `json:"sfp_id"`
		SfpIPAddress    string `json:"sfp_ip_address"`
		SfpMacAddress   string `json:"sfp_mac_address"`
		StreamID        uint32 `json:"stream_id"`
		TimeEpoch       int32  `json:"time_epoch"`
	} `json:"iq_stream"`
	SampleRateSps      json.Number `json:"sample_rate_sps"` // Changed from uint64 to json.Number to handle both int and float values during unmarshalling
	SnapshotNumSamples uint64      `json:"snapshot_num_samples"`
	TriggerStartTimeNs uint64      `json:"trigger_start_time_ns"`
	TriggerType        int32       `json:"trigger_type"`
	TunerID            uint32      `json:"tuner_id"`
	UniqueID           string      `json:"unique_id"`
}

// NewDdcCollector initializes a DdcCollector with default values or values provided by the caller.
func NewDdcCollector(destIp string, destPort uint32, macAddress string, tunerID uint32) DdcCollector {
	return DdcCollector{
		AGCSettings: struct {
			Attenuation_dB float64 `json:"attenuation_dB"`
			Enabled        bool    `json:"enable"`
		}{
			Attenuation_dB: 0,
			Enabled:        true,
		},
		BandwidthHz:       80000000,
		CenterFreqHz:      150000000,
		CollectorID:       tunerID,
		EnablePreamp:      true,
		FrequencyOffsetHz: 0,
		IQStream: struct {
			DucBufferSizeMb uint32 `json:"duc_buffer_size_mb"`
			Encoding        int32  `json:"encoding"`
			HostName        string `json:"host_name"`
			LocalPort       uint32 `json:"local_port"`
			MacAddress      string `json:"mac_address"`
			PayloadSize     uint32 `json:"payload_size"`
			Port            uint32 `json:"port"`
			SfpID           uint32 `json:"sfp_id"`
			SfpIPAddress    string `json:"sfp_ip_address"`
			SfpMacAddress   string `json:"sfp_mac_address"`
			StreamID        uint32 `json:"stream_id"`
			TimeEpoch       int32  `json:"time_epoch"`
		}{
			DucBufferSizeMb: 0,
			Encoding:        3,
			HostName:        destIp,
			LocalPort:       destPort,
			MacAddress:      macAddress,
			PayloadSize:     2048,
			Port:            destPort,
			SfpID:           0,
			SfpIPAddress:    "dummy_sfp_ip",
			SfpMacAddress:   "dummy_sfp_mac",
			StreamID:        tunerID,
			TimeEpoch:       0,
		},
		SampleRateSps:      json.Number("125000000"),
		SnapshotNumSamples: 0,
		TriggerStartTimeNs: 0,
		TriggerType:        0,
		TunerID:            tunerID,
		UniqueID:           "t2_ltu",
	}
}

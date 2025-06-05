package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"luf.co/lsu/pkg/driver"
	"luf.co/lsu/pkg/driver/uhd_holoscan"
)

var (
	availableDrivers = map[string]func() driver.SDRDriver{
		"uhd":          func() driver.SDRDriver { return &uhd.UHDDriver{} },
		"uhd_holoscan": func() driver.SDRDriver { return &uhd_holoscan.UHDHoloscanDriver{} },
	}
	selectedDriver driver.SDRDriver
	driverName     = "uhd_holoscan" // Default driver
)

type Command struct {
	Name string
	Args []string
}

func main() {
	// Create channels for commands and done signal
	commands := make(chan Command)
	done := make(chan struct{})

	// Start the device driver goroutine
	go commandLoop(commands, done)

	// Simple CLI loop
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Logical Sensor Unit (LSU) CLI - type 'help' or 'exit'")

	for {
		fmt.Print("> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Error reading input: %v\n", err)
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		cmdName := parts[0]
		var cmdArgs []string
		if len(parts) > 1 {
			cmdArgs = parts[1:]
		}

		// Send command to driver
		commands <- Command{Name: cmdName, Args: cmdArgs}

		if cmdName == "exit" {
			// Wait for driver to confirm it's shutting down
			<-done
			fmt.Println("Exiting.")
			return
		}
	}
}

func commandLoop(commands <-chan Command, done chan<- struct{}) error {
	var dev driver.SDRDriver //  SDR device

	// Initialize the selected driver
	if initFunc, ok := availableDrivers[driverName]; ok {
		selectedDriver = initFunc()
		dev = selectedDriver
		fmt.Printf("Using driver: %s\n", driverName)
	} else {
		fmt.Printf("Error: Driver '%s' not found.\n", driverName)
		os.Exit(1) // Exit if the driver is not found
		return fmt.Errorf("driver not found")
	}

	// Check if the driver was correctly initialized
	if dev == nil {
		fmt.Println("Driver initialization failed.")
		os.Exit(1)
		return fmt.Errorf("driver initialization failed")
	}

	for cmd := range commands {
		switch cmd.Name {
		case "help":
			handleHelp()

		case "init":
			err := handleInit(dev, cmd.Args)
			if err != nil {
				fmt.Printf("Init failed: %v\n", err)
			}

		case "get_num_tuners":
			handleGetNumTuners(dev)

		case "get_num_coherent_tuners":
			handleGetNumCoherentTuners(dev)

		case "get_tuning_granularity":
			handleGetTuningGranularity(dev)

		case "get_status":
			handleGetStatus(dev)

		case "tuner_get_center_frequency":
			err := handleTunerGetCenterFrequency(dev, cmd.Args)
			if err != nil {
				fmt.Printf("Get tuner center frequency failed: %v\n", err)
			}

		case "tuner_set_center_frequency":
			err := handleTunerSetCenterFrequency(dev, cmd.Args)
			if err != nil {
				fmt.Printf("Set tuner center frequency failed: %v\n", err)
			}

		case "tuner_get_useable_bandwidth":
			err := handleTunerGetUseableBandwidth(dev, cmd.Args)
			if err != nil {
				fmt.Printf("Get tuner useable bandwidth failed: %v\n", err)
			}

		case "tuner_get_current_bandwidth":
			err := handleTunerGetCurrentBandwidth(dev, cmd.Args)
			if err != nil {
				fmt.Printf("Get tuner current bandwidth failed: %v\n", err)
			}

		case "tuner_set_current_bandwidth":
			err := handleTunerSetCurrentBandwidth(dev, cmd.Args)
			if err != nil {
				fmt.Printf("Set tuner current bandwidth failed: %v\n", err)
			}

		case "tuner_get_sample_rate":
			err := handleTunerGetSampleRate(dev, cmd.Args)
			if err != nil {
				fmt.Printf("Get tuner sample rate failed: %v\n", err)
			}

		case "tuner_set_sample_rate":
			err := handleTunerSetSampleRate(dev, cmd.Args)
			if err != nil {
				fmt.Printf("Set tuner sample rate failed: %v\n", err)
			}

		case "tuner_get_gain":
			err := handleTunerGetGain(dev, cmd.Args)
			if err != nil {
				fmt.Printf("Get tuner gain failed: %v\n", err)
			}

		case "tuner_set_gain":
			err := handleTunerSetGain(dev, cmd.Args)
			if err != nil {
				fmt.Printf("Set tuner gain failed: %v\n", err)
			}

		case "tuner_get_dc_bias":
			err := handleTunerGetDcBias(dev, cmd.Args)
			if err != nil {
				fmt.Printf("Get tuner DC bias failed: %v\n", err)
			}

		case "tuner_set_dc_bias":
			err := handleTunerSetDcBias(dev, cmd.Args)
			if err != nil {
				fmt.Printf("Set tuner DC bias failed: %v\n", err)
			}

		case "tuner_get_agc":
			err := handleTunerGetAgc(dev, cmd.Args)
			if err != nil {
				fmt.Printf("Get tuner AGC failed: %v\n", err)
			}

		case "tuner_set_agc":
			err := handleTunerSetAgc(dev, cmd.Args)
			if err != nil {
				fmt.Printf("Set tuner AGC failed: %v\n", err)
			}

		case "tuner_start":
			err := handleTunerStart(dev, cmd.Args)
			if err != nil {
				fmt.Printf("Tuner start failed: %v\n", err)
			}

		case "tuner_stop":
			err := handleTunerStop(dev, cmd.Args)
			if err != nil {
				fmt.Printf("Tuner stop failed: %v\n", err)
			}

		case "exit":
			handleExit(dev, done)
			return nil

		default:
			fmt.Printf("Unknown command: %s\n", cmd.Name)
		}
	}
	return nil
}

func handleGetNumTuners(dev driver.SDRDriver) {
	numTuners := dev.GetNumTuners()
	fmt.Printf("Number of tuners: %d\n", numTuners)
}

func handleGetNumCoherentTuners(dev driver.SDRDriver) {
	numCoherentTuners := dev.GetNumCoherentTuners()
	fmt.Printf("Number of coherent tuners: %d\n", numCoherentTuners)
}

func handleGetTuningGranularity(dev driver.SDRDriver) {
	granularity := dev.GetTuningGranularity()
	fmt.Printf("Tuning Granularity - Boundary: %.2f Hz, Precision: %.2f\n", granularity.BoundaryHz, granularity.Precision)
}

func handleGetStatus(dev driver.SDRDriver) {
	status := dev.GetStatus()
	fmt.Printf("SDR Status - Alive: %t, Latitude: %.2f, Longitude: %.2f, Elevation: %.2f\n", status.Alive, status.Position.Latitude, status.Position.Longitude, status.Position.Elevation)
}

func getTunerIndex(args []string) (int, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("expected tuner index as argument")
	}
	index, err := strconv.Atoi(args[0])
	if err != nil {
		return 0, fmt.Errorf("invalid tuner index: %v", err)
	}
	return index, nil
}

func handleTunerGetCenterFrequency(dev driver.SDRDriver, args []string) error {
	index, err := getTunerIndex(args)
	if err != nil {
		return err
	}

	tuners := dev.GetTuners()
	if index < 0 || index >= len(tuners) {
		return fmt.Errorf("tuner index out of range")
	}

	freq := tuners[index].GetCenterFrequency()
	fmt.Printf("Tuner %d Center Frequency: %d Hz\n", index, freq)
	return nil
}

func handleTunerSetCenterFrequency(dev driver.SDRDriver, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("expected tuner index and frequency as arguments")
	}

	index, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid tuner index: %v", err)
	}

	freq, err := strconv.ParseUint(args[1], 10, 32)
	if err != nil {
		return fmt.Errorf("invalid frequency: %v", err)
	}

	tuners := dev.GetTuners()
	if index < 0 || index >= len(tuners) {
		return fmt.Errorf("tuner index out of range")
	}

	err = tuners[index].SetCenterFrequency(uint32(freq))
	if err != nil {
		return err
	}

	fmt.Printf("Tuner %d Center Frequency set to: %d Hz\n", index, freq)
	return nil
}

func handleTunerGetUseableBandwidth(dev driver.SDRDriver, args []string) error {
	index, err := getTunerIndex(args)
	if err != nil {
		return err
	}

	tuners := dev.GetTuners()
	if index < 0 || index >= len(tuners) {
		return fmt.Errorf("tuner index out of range")
	}

	bandwidth := tuners[index].GetUseableBandwidth()
	fmt.Printf("Tuner %d Useable Bandwidth: %d Hz\n", index, bandwidth)
	return nil
}

func handleTunerGetCurrentBandwidth(dev driver.SDRDriver, args []string) error {
	index, err := getTunerIndex(args)
	if err != nil {
		return err
	}

	tuners := dev.GetTuners()
	if index < 0 || index >= len(tuners) {
		return fmt.Errorf("tuner index out of range")
	}

	bandwidth := tuners[index].GetCurrentBandwidth()
	fmt.Printf("Tuner %d Current Bandwidth: %d Hz\n", index, bandwidth)
	return nil
}

func handleTunerSetCurrentBandwidth(dev driver.SDRDriver, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("expected tuner index and bandwidth as arguments")
	}

	index, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid tuner index: %v", err)
	}

	bandwidth, err := strconv.ParseUint(args[1], 10, 32)
	if err != nil {
		return fmt.Errorf("invalid bandwidth: %v", err)
	}

	tuners := dev.GetTuners()
	if index < 0 || index >= len(tuners) {
		return fmt.Errorf("tuner index out of range")
	}

	err = tuners[index].SetCurrentBandwidth(uint32(bandwidth))
	if err != nil {
		return err
	}

	fmt.Printf("Tuner %d Current Bandwidth set to: %d Hz\n", index, bandwidth)
	return nil
}

func handleTunerGetSampleRate(dev driver.SDRDriver, args []string) error {
	index, err := getTunerIndex(args)
	if err != nil {
		return err
	}

	tuners := dev.GetTuners()
	if index < 0 || index >= len(tuners) {
		return fmt.Errorf("tuner index out of range")
	}

	rate := tuners[index].GetSampleRate()
	fmt.Printf("Tuner %d Sample Rate: %d Hz\n", index, rate)
	return nil
}

func handleTunerSetSampleRate(dev driver.SDRDriver, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("expected tuner index and sample rate as arguments")
	}

	index, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid tuner index: %v", err)
	}

	rate, err := strconv.ParseUint(args[1], 10, 32)
	if err != nil {
		return fmt.Errorf("invalid sample rate: %v", err)
	}

	tuners := dev.GetTuners()
	if index < 0 || index >= len(tuners) {
		return fmt.Errorf("tuner index out of range")
	}

	err = tuners[index].SetSampleRate(uint32(rate))
	if err != nil {
		return err
	}

	fmt.Printf("Tuner %d Sample Rate set to: %d Hz\n", index, rate)
	return nil
}

func handleTunerGetGain(dev driver.SDRDriver, args []string) error {
	index, err := getTunerIndex(args)
	if err != nil {
		return err
	}

	tuners := dev.GetTuners()
	if index < 0 || index >= len(tuners) {
		return fmt.Errorf("tuner index out of range")
	}

	gain := tuners[index].GetGain()
	fmt.Printf("Tuner %d Gain: %d\n", index, gain)
	return nil
}

func handleTunerSetGain(dev driver.SDRDriver, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("expected tuner index and gain as arguments")
	}

	index, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid tuner index: %v", err)
	}

	gain, err := strconv.ParseUint(args[1], 10, 32)
	if err != nil {
		return fmt.Errorf("invalid gain: %v", err)
	}

	tuners := dev.GetTuners()
	if index < 0 || index >= len(tuners) {
		return fmt.Errorf("tuner index out of range")
	}

	err = tuners[index].SetGain(uint32(gain))
	if err != nil {
		return err
	}

	fmt.Printf("Tuner %d Gain set to: %d\n", index, gain)
	return nil
}

func handleTunerGetDcBias(dev driver.SDRDriver, args []string) error {
	index, err := getTunerIndex(args)
	if err != nil {
		return err
	}

	tuners := dev.GetTuners()
	if index < 0 || index >= len(tuners) {
		return fmt.Errorf("tuner index out of range")
	}

	dcBias := tuners[index].GetDcBias()
	fmt.Printf("Tuner %d DC Bias: %t\n", index, dcBias)
	return nil
}

func handleTunerSetDcBias(dev driver.SDRDriver, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("expected tuner index and DC bias value as arguments")
	}

	index, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid tuner index: %v", err)
	}

	dcBias, err := strconv.ParseBool(args[1])
	if err != nil {
		return fmt.Errorf("invalid DC bias value: %v", err)
	}

	tuners := dev.GetTuners()
	if index < 0 || index >= len(tuners) {
		return fmt.Errorf("tuner index out of range")
	}

	err = tuners[index].SetDcBias(dcBias)
	if err != nil {
		return err
	}

	fmt.Printf("Tuner %d DC Bias set to: %t\n", index, dcBias)
	return nil
}

func handleTunerGetAgc(dev driver.SDRDriver, args []string) error {
	index, err := getTunerIndex(args)
	if err != nil {
		return err
	}

	tuners := dev.GetTuners()
	if index < 0 || index >= len(tuners) {
		return fmt.Errorf("tuner index out of range")
	}

	agc := tuners[index].GetAgc()
	fmt.Printf("Tuner %d AGC: %t\n", index, agc)
	return nil
}

func handleTunerSetAgc(dev driver.SDRDriver, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("expected tuner index and AGC value as arguments")
	}

	index, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid tuner index: %v", err)
	}

	agc, err := strconv.ParseBool(args[1])
	if err != nil {
		return fmt.Errorf("invalid AGC value: %v", err)
	}

	tuners := dev.GetTuners()
	if index < 0 || index >= len(tuners) {
		return fmt.Errorf("tuner index out of range")
	}

	err = tuners[index].SetAgc(agc)
	if err != nil {
		return err
	}

	fmt.Printf("Tuner %d AGC set to: %t\n", index, agc)
	return nil
}

func handleTunerStart(dev driver.SDRDriver, args []string) error {
	index, err := getTunerIndex(args)
	if err != nil {
		return err
	}

	tuners := dev.GetTuners()
	if index < 0 || index >= len(tuners) {
		return fmt.Errorf("tuner index out of range")
	}

	tuners[index].Start()
	return nil
}

func handleTunerStop(dev driver.SDRDriver, args []string) error {
	index, err := getTunerIndex(args)
	if err != nil {
		return err
	}

	tuners := dev.GetTuners()
	if index < 0 || index >= len(tuners) {
		return fmt.Errorf("tuner index out of range")
	}

	tuners[index].Stop()
	return nil
}

func handleHelp() {
	fmt.Println("Available commands:")
	fmt.Println("  init [args] - Initialize the SDR device")
	fmt.Println("  get_num_tuners - Get the number of tuners")
	fmt.Println("  get_num_coherent_tuners - Get the number of coherent tuners")
	fmt.Println("  get_tuning_granularity - Get the tuning granularity")
	fmt.Println("  get_status - Get the SDR status")

	fmt.Println("  tuner_get_center_frequency [index] - Get the center frequency of a tuner")
	fmt.Println("  tuner_set_center_frequency [index] [frequency] - Set the center frequency of a tuner")
	fmt.Println("  tuner_get_useable_bandwidth [index] - Get the useable bandwidth of a tuner")
	fmt.Println("  tuner_get_current_bandwidth [index] - Get the current bandwidth of a tuner")
	fmt.Println("  tuner_set_current_bandwidth [index] [bandwidth] - Set the current bandwidth of a tuner")
	fmt.Println("  tuner_get_sample_rate [index] - Get the sample rate of a tuner")
	fmt.Println("  tuner_set_sample_rate [index] [rate] - Set the sample rate of a tuner")
	fmt.Println("  tuner_get_gain [index] - Get the gain of a tuner")
	fmt.Println("  tuner_set_gain [index] [gain] - Set the gain of a tuner")
	fmt.Println("  tuner_get_dc_bias [index] - Get the DC bias of a tuner")
	fmt.Println("  tuner_set_dc_bias [index] [true|false] - Set the DC bias of a tuner")
	fmt.Println("  tuner_get_agc [index] - Get the AGC of a tuner")
	fmt.Println("  tuner_set_agc [index] [true|false] - Set the AGC of a tuner")
	fmt.Println("  tuner_start [index] - Start a tuner")
	fmt.Println("  tuner_stop [index] - Stop a tuner")

	fmt.Println("  exit - Exit the CLI")
}

func handleInit(dev driver.SDRDriver, args []string) error {
	fmt.Println("Initializing the device...")
	err := dev.Init(args)
	if err != nil {
		fmt.Printf("Initialization error: %v\n", err)
		return err
	}
	fmt.Println("Device initialized successfully.")
	return nil
}

func handleExit(dev driver.SDRDriver, done chan<- struct{}) {
	fmt.Println("Exiting...")
	err := dev.Close()
	if err != nil {
		fmt.Printf("Error closing device: %v\n", err)
	}
	done <- struct{}{} // Signal that the driver has finished
}

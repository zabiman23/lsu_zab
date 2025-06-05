
#include <iostream>

#include <ctype.h>
#include <ctime>
#include <stdio.h>
#include <stdint.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>
#include <signal.h>
#include <errno.h>
#include <unistd.h>
#include <inttypes.h>
#include <fcntl.h>
#include <array>
#include <algorithm>
#include <thread>
#include <mutex>
#include <vector>
#include <map>

// CUDA includes
#include <cuda_runtime.h>
#include <cufft.h> // Main cuFFT header

#include "sidekiq_api.h"
#include "gps_utils.h"
#include "x40_driver.h"
#include "x40_constants.hpp"
#include "circular_buffer.hpp"

constexpr int BUFFER_CAPACITY = 4096;
constexpr int X40_TUNER_COUNT = skiq_rx_hdl_end;
#define  SCALE (1.0 / pow(2,15));

//--------------------------------------------------------------------
// CUDA SUPPORT FUNCTIONS
//--------------------------------------------------------------------

// --- CUDA Error Handling Macro/Function (Highly Recommended!) ---
// Basic error checking helper
inline void gpuAssert(cudaError_t code, const char *file, int line, bool abort = true) {
    if (code != cudaSuccess) {
        fprintf(stderr, "GPUassert: %s %s %d\n", cudaGetErrorString(code), file, line);
        if (abort) exit(code);
    }
}
// Macro to wrap CUDA calls
#define CUDA_CHECK(ans) { gpuAssert((ans), __FILE__, __LINE__); }

// Basic cuFFT error checking helper
inline void cufftAssert(cufftResult code, const char *file, int line, bool abort = true) {
    if (code != CUFFT_SUCCESS) {
        // Note: cuFFT error strings are not as standardized as CUDA runtime ones.
        // You might need a lookup table or just print the error code.
        fprintf(stderr, "cuFFTassert: error code %d %s %d\n", code, file, line);
        if (abort) exit(code);
    }
}
// Macro to wrap cuFFT calls
#define CUFFT_CHECK(ans) { cufftAssert((ans), __FILE__, __LINE__); }

// --- Kernel 1: Calculate Log Power (dB) ---
// Calculates 10 * log10(real^2 + imag^2 + epsilon), applying a floor
__global__ void calculateLogPowerKernel(const cufftComplex* complex_fft_in, float* power_db_out, int N, float epsilon, float db_floor) {
  int idx = blockIdx.x * blockDim.x + threadIdx.x;

  if (idx < N) {
      float re = complex_fft_in[idx].x;
      float im = complex_fft_in[idx].y;
      float power_lin = re * re + im * im;
      // Add epsilon to prevent log10(0) -> -inf
      float power_db = 10.0f * log10f(power_lin + epsilon);
      // Apply floor to prevent extremely low values if desired
      power_db_out[idx] = fmaxf(db_floor, power_db);
  }
}

// --- Kernel 2: Perform FFT Shift ---
// Rearranges the input array so that DC component is in the center.
// Assumes N is the FFT size (must be even for this simple shift).
__global__ void fftShiftKernel(const float* power_db_in, float* power_db_shifted_out, int N) {
  int idx = blockIdx.x * blockDim.x + threadIdx.x;
  int half_N = N / 2; // Assumes N is even

  if (idx < N) {
      // Calculate the source index after shift
      // Elements from [N/2, N-1] move to [0, N/2-1]
      // Elements from [0, N/2-1] move to [N/2, N-1]
      int shifted_idx = (idx + half_N) % N;
      power_db_shifted_out[idx] = power_db_in[shifted_idx];
  }
}

/**
 * CUDA kernel to convert interleaved short I/Q data to complex float data.
 *
 * Each thread processes one I/Q pair from the input array and writes one
 * complex float (represented as float2) to the output array.
 * Assumes input is [I0, Q0, I1, Q1,...] and output is [C0, C1,...],
 * where Ck.x = (float)Ik and Ck.y = (float)Qk.
 *
 * @param input Pointer to the input array of interleaved short integers in global memory.
 * @param output Pointer to the output array of float2 complex numbers in global memory.
 * @param num_complex_samples The total number of complex samples to process (size of the output array).
 */
__global__ void interleavedShortToComplexFloat(const short* input, float2* output, int num_complex_samples) {
    // Calculate the global thread index. This index corresponds directly
    // to the index of the complex sample (and the output float2 element)
    // this thread will compute. The calculation ensures linear mapping across blocks.
    int idx = blockIdx.x * blockDim.x + threadIdx.x;

    // Bounds check: Ensure the calculated index is within the valid range
    // of complex samples. This prevents threads from accessing memory
    // outside the allocated input/output arrays, which can happen if the
    // total number of threads launched exceeds num_complex_samples.
    if (idx < num_complex_samples) {
        // Calculate the starting index for the corresponding I/Q pair in the input array.
        // Since each complex sample corresponds to two short values (I and Q),
        // the input index is twice the complex sample index.
        int input_idx = 2 * idx;

        // Read the interleaved short I and Q values from global memory.
        // These two reads access consecutive memory locations (input[input_idx]
        // and input[input_idx + 1]). Across threads in a warp, these accesses
        // target a contiguous block of memory, enabling coalesced reads.
        //
        short i_short = input[input_idx];     // Read I component (at 2*idx)
        short q_short = input[input_idx + 1]; // Read Q component (at 2*idx + 1)

        // Convert the short integer values to single-precision floats.
        // static_cast is a standard and efficient way to perform this conversion.
        //
        float i_float = static_cast<float>(i_short);
        float q_float = static_cast<float>(q_short);

        // Construct the float2 complex representation.
        // The.x member holds the real part (I component).
        // The.y member holds the imaginary part (Q component).
        //
        float2 complex_val;
        complex_val.x = i_float * SCALE;
        complex_val.y = q_float * SCALE;

        // Write the resulting float2 (complex float) to the output array
        // in global memory. Since consecutive threads write to consecutive
        // indices (output[idx], output[idx+1],...), and float2 is an 8-byte
        // structure, these writes are coalesced.
        //
        output[idx] = complex_val;
    }
}

//--------------------------------------------------------------------
// END CUDA SUPPORT FUNCTIONS
//--------------------------------------------------------------------

/**
 * @brief Struct to hold driver data and state.
 * TODO - either make this entire driver object-oriented or make 
 * this struct literally only hold data and implement all methods C-style.
 * I know this current setup is janky but I wanted to get something working
 */
typedef struct X40DriverData {   
    uint8_t cardNum;
    std::string cardSerial;     
    const char *zynqIP;
    const char *zynqGPSDPort;
    std::thread *gpsThread;
    GPSInfo gpsInfo;
    volatile bool keepRunning;
    volatile bool keepReading;    
    bool coherent_mode;
    std::array<X40DataBuffer*, X40_TUNER_COUNT> rxBuffers; // Circular buffers for each tuner
    std::mutex streamMutex;
    std::array<std::thread*, X40_TUNER_COUNT> tunerRxProcThreads;
    std::thread *rxMainThread;
    std::vector<std::vector<float>> powerDbs;     
    std::vector<std::mutex> powerDbsMutexes; 
    std::map<int, bool> activeTuners;
    bool isRxMainThreadActive;
    X40DriverData(uint8_t cNum, char *cSerial, const char *zynqIP, const char *zynqGPSDPort): 
             cardNum(cNum), cardSerial(cSerial), zynqIP(zynqIP), zynqGPSDPort(zynqGPSDPort),
             keepRunning(true), keepReading(true), coherent_mode(false), gpsThread(nullptr), 
             rxMainThread(nullptr),
             powerDbs(X40_TUNER_COUNT, std::vector<float>(PKT_LEN/2, 0.0f)), isRxMainThreadActive(false)
             ,powerDbsMutexes(X40_TUNER_COUNT)
    {
        // Initialize circular buffers for each tuner
        for (int i = 0; i < X40_TUNER_COUNT; ++i) {
            X40DataBuffer *buf = new X40DataBuffer(BUFFER_CAPACITY);
            rxBuffers[i] = buf;
        }
        // Initialize the activeTuners map
        for (int i = 0; i < X40_TUNER_COUNT; ++i) {
            activeTuners[i] = false; 
        }
        for (int i = 0; i < X40_TUNER_COUNT; ++i) {
            tunerRxProcThreads[i] = nullptr; 
        }
    }
    ~X40DriverData(){
        keepRunning = false;
        if(gpsThread != nullptr){            
            gpsThread->join();            
            delete gpsThread;
        }
        // stop rx proc threads
        for (int i = 0; i < X40_TUNER_COUNT; i++) {
            std::thread *procThread = tunerRxProcThreads[i];
            if ( procThread != nullptr) {               
                procThread->join();                
                delete procThread;
            }
        }
        // delete circular buffers
        for (int i = 0; i < X40_TUNER_COUNT; ++i) {
            delete rxBuffers[i];
        }
    }
    
} X40DriverData;

//--------------------------------------------------------------------
// HELPER AND NON-API FUNCTION DECLARATIONS
//--------------------------------------------------------------------

/**
 * Gets the PROGRAMMED sample rate and bandwidth for the given tuner.
 */
uint32_t getProgrammedSRandBW(uint8_t cardNum, skiq_rx_hdl_t tunerHdl, uint32_t *sr, uint32_t *bw);

/**
 * Gets the ACTUAL sample rate and bandwidth for the given tuner.
 */
uint32_t getActualSRandBW(uint8_t cardNum, skiq_rx_hdl_t tunerHdl, double *sr, uint32_t *bw);

void getGainRangeSettings(uint8_t cardNum, skiq_rx_hdl_t tunerHdl);

uint8_t getGainIdxFromGainDB(uint32_t gainValDB, uint8_t idxMin, uint8_t idxMax);

uint32_t getGainDBFromGainIdx(uint8_t idx, uint8_t idxMin, uint8_t idxMax);

void startGPSPolling(X40DriverData *driverdata);

void processRxDataForTuner(X40DriverData *driverdata, skiq_rx_hdl_t tunerHdl);

void readRxData(X40DriverData *driverdata);

void startMainRxReadThread(X40DriverData *driverdata);

/**
 * Stops the main RX read thread, which is responsible for reading data from the tuners.
 * Does not stop the RX processing threads.
 */
void stopMainRxReadThread(X40DriverHandle handle);

/**
 * Stops the RX processing threads; does not stop the main RX read thread.
 */
void stopRxProcessingThreads(X40DriverHandle handle);

/**
 * Stops ALL threads, should only be called before deleting the X40DriverData struct.
 */
void stopAllThreads(X40DriverHandle handle);

/**
 * Restarts the RX stream for the specified tuner. Performs this at the SKIQ
 * level ONLY; does not restart the RX read thread or the RX processing thread.
 * Does nothing if the stream is not running. Pauses for 1ms after stoping the stream
 */
void skiqSafeRestartRxStream(X40DriverData *driverdata, skiq_rx_hdl_t tunerHdl);

//--------------------------------------------------------------------
// API FUNCTION IMPLEMENTATIONS
//--------------------------------------------------------------------

X40DriverHandle X40Driver_Init(const char* args) {
    std::cout << "X40Driver_Init called with args: " << args << std::endl;
    
    uint8_t cardNum = 0;
    char *serialStr;
    skiq_read_serial_string(cardNum, &serialStr );
    skiq_xport_type_t xpType = skiq_xport_type_auto;
    skiq_xport_init_level_t xpInitLevel = skiq_xport_init_level_full;
    // there's just one card on the X40 and it's card 0
    
    int32_t retVal = skiq_init(xpType, xpInitLevel, &cardNum, 1);
    if(retVal != 0){
        return nullptr;
    }
    // X40 defaults to Q/I mode, we want I/Q mode
    skiq_write_iq_order_mode(cardNum, skiq_iq_order_iq);   
     
    X40DriverData *driverHandle = new X40DriverData(cardNum, serialStr, ZYNQ_IP, ZYNQ_GPSDO_PORT);  
    startGPSPolling(driverHandle);

    // Start RX streaming right away
    for(int i = 0; i < X40_TUNER_COUNT; i++) {
        X40Driver_StartRxStream(driverHandle, i);
        std::this_thread::sleep_for(std::chrono::microseconds(500));
    }
    return (X40DriverHandle) driverHandle;
}



void X40Driver_Close(X40DriverHandle handle) {
    if(handle != nullptr) {
        std::cout << "X40Driver_Close called" << std::endl;
        X40DriverData *driverdata = (X40DriverData*)handle;
        stopAllThreads(handle);
        skiq_exit();        
        delete driverdata; 
    } else {
        std::cout << "X40Driver_Close called with null handle" << std::endl;
        return; // Nothing to close
    }    
}

uint32_t X40Driver_GetNumTuners(X40DriverHandle handle) {
    /*
    skiq_rx_hdl_A1=0,
    skiq_rx_hdl_A2=1,
    skiq_rx_hdl_B1=2,
    skiq_rx_hdl_B2=3,
    skiq_rx_hdl_C1=4,
    skiq_rx_hdl_D1=5,
    skiq_rx_hdl_end
     */
    std::cout << "X40Driver_GetNumTuners called" << std::endl;
    // Implement logic to get the number of tuners
    return X40_TUNER_COUNT; //skiq_rx_hdl_end; 
}

uint32_t X40Driver_GetNumCoherentTuners(X40DriverHandle handle) {
    std::cout << "X40Driver_GetNumCoherentTuners called" << std::endl;
    // Coherent are A1+A2, and B1+B2 according to the docs
    return 4; 
}

uint32_t X40Driver_GetRxFrequency(X40DriverHandle handle, uint32_t tunerIndex) {
    if(handle == nullptr) {
        std::cout << "X40Driver_GetRxFrequency called with null handle" << std::endl;
        return 0; // Invalid handle
    }
    std::cout << "X40Driver_GetRxFrequency called with tunerIndex: " << tunerIndex << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    uint64_t programmedFreq;
    double actualFreq;
    int32_t retVal = skiq_read_rx_LO_freq(driverdata->cardNum, tunerIndexHdl, &programmedFreq, &actualFreq);
    return (uint32_t)actualFreq;
}

void X40Driver_SetRxFrequency(X40DriverHandle handle, uint32_t tunerIndex, uint32_t freq) {
    if(handle == nullptr) {
        std::cout << "X40Driver_SetRxFrequency called with null handle" << std::endl;
        return;
    }
    std::cout << "X40Driver_SetRxFrequency called with tunerIndex: " << tunerIndex << " and freq: " << freq << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    uint32_t retVal = skiq_write_rx_LO_freq(driverdata->cardNum, tunerIndexHdl, (uint64_t) freq);
    if(retVal == 0){
        skiqSafeRestartRxStream(driverdata, tunerIndexHdl);        
    }
    else{
        std::cout << "Error: Setting RX frequency failed with error code " << retVal << std::endl;
    }
}

uint32_t X40Driver_GetSampleRate(X40DriverHandle handle, uint32_t tunerIndex) {
    if(handle == nullptr) {
        std::cout << "X40Driver_GetSampleRate called with null handle" << std::endl;
        return 0; // Invalid handle
    }
    std::cout << "X40Driver_GetSampleRate called with tunerIndex: " << tunerIndex << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    uint32_t programmedRate;
    double actualRate;
    int32_t retVal = skiq_read_rx_sample_rate(driverdata->cardNum, tunerIndexHdl, &programmedRate, &actualRate);
    std::cout << "Programmed Sample Rate for Tuner " << tunerIndex << ": " << programmedRate << std::endl;
    std::cout << "Actual Sample Rate for Tuner " << tunerIndex << ": " << actualRate << std::endl;
    return (uint32_t)actualRate;
}

void X40Driver_SetSampleRate(X40DriverHandle handle, uint32_t tunerIndex, uint32_t rate) {
    if(handle == nullptr) {
        std::cout << "X40Driver_SetSampleRate called with null handle" << std::endl;
        return ;
    }
    std::cout << "X40Driver_SetSampleRate called with tunerIndex: " << tunerIndex << " and rate: " << rate << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    // API doesn't let us set SR by itself, have to set BW too. So let's use the currently configured BW
    uint32_t oldSR, oldBW;
    uint32_t retVal = getProgrammedSRandBW(driverdata->cardNum, tunerIndexHdl, &oldSR, &oldBW);
    if(retVal != 0){
        std::cout << "Error: Fetching sample rate and bandwidth failed with error code " << retVal << std::endl;
        return;
    }
    retVal = skiq_write_rx_sample_rate_and_bandwidth(driverdata->cardNum, tunerIndexHdl, rate, oldBW);
    if(retVal == 0){
        skiqSafeRestartRxStream(driverdata, tunerIndexHdl);        
    }
    else{
        std::cout << "Error: Setting sample rate failed with error code " << retVal << std::endl;
    }
}

uint32_t X40Driver_GetRxBandwidth(X40DriverHandle handle, uint32_t tunerIndex) {
    if(handle == nullptr) {
        std::cout << "X40Driver_GetRxBandwidth called with null handle" << std::endl;
        return 0; // Invalid handle
    }
    std::cout << "X40Driver_GetRxBandwidth called with tunerIndex: " << tunerIndex << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    double sr;
    uint32_t bw;
    uint32_t retVal = getActualSRandBW(driverdata->cardNum, tunerIndexHdl, &sr, &bw);
    if(retVal != 0){
        std::cout << "Error: Fetching sample rate and bandwidth failed with error code " << retVal << std::endl;
    }
    return bw;
}

void X40Driver_SetRxBandwidth(X40DriverHandle handle, uint32_t tunerIndex, uint32_t bandwidth) {
    if(handle == nullptr) {
        std::cout << "X40Driver_SetRxBandwidth called with null handle" << std::endl;
        return; // Invalid handle
    }
    std::cout << "X40Driver_SetRxBandwidth called with tunerIndex: " << tunerIndex << " and bandwidth: " << bandwidth << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    // API doesn't let us set BW by itself, have to set SR too. So let's use the currently configured SR
    uint32_t oldSR, oldBW;
    uint32_t retVal = getProgrammedSRandBW(driverdata->cardNum, tunerIndexHdl, &oldSR, &oldBW);
    if(retVal != 0){
        std::cout << "Error: Fetching sample rate and bandwidth failed with error code " << retVal << std::endl;
        return;
    }
    retVal = skiq_write_rx_sample_rate_and_bandwidth(driverdata->cardNum, tunerIndexHdl, oldSR, bandwidth);
    if(retVal == 0){
        skiqSafeRestartRxStream(driverdata, tunerIndexHdl);        
    }
    else{
        std::cout << "Error: Setting sample rate failed with error code " << retVal << std::endl;
    }
}

uint32_t X40Driver_GetGain(X40DriverHandle handle, uint32_t tunerIndex) {
    if(handle == nullptr) {
        std::cout << "X40Driver_GetGain called with null handle" << std::endl;
        return 0; // Invalid handle
    }
    std::cout << "X40Driver_GetGain called with tunerIndex: " << tunerIndex << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    uint8_t gainIdxMin, gainIdxMax, gainIdx; 
    skiq_read_rx_gain_index_range (driverdata->cardNum, tunerIndexHdl, &gainIdxMin, &gainIdxMax);
    std::cout << "Gain range: " << static_cast<uint32_t>(gainIdxMin) << " - " << static_cast<uint32_t>(gainIdxMax) << std::endl;
    skiq_read_rx_gain(driverdata->cardNum, tunerIndexHdl, &gainIdx);
    std::cout << "Reported gain index: " << static_cast<uint32_t>(gainIdx) << std::endl;
    return getGainDBFromGainIdx(gainIdx, gainIdxMin, gainIdxMax);    
}

void X40Driver_SetGain(X40DriverHandle handle, uint32_t tunerIndex, uint32_t gain) {
    if(handle == nullptr) {
        std::cout << "X40Driver_SetGain called with null handle" << std::endl;
        return; // Invalid handle
    }
    std::cout << "X40Driver_SetGain called with tunerIndex: " << tunerIndex << " and gain: " << gain << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    uint8_t gainIdxMin, gainIdxMax, gainIdx; 
    skiq_read_rx_gain_index_range (driverdata->cardNum, tunerIndexHdl, &gainIdxMin, &gainIdxMax);
    gainIdx = getGainIdxFromGainDB(gain, gainIdxMin, gainIdxMax);
    std::cout << "Derived gain index: " << static_cast<uint32_t>(gainIdx) << std::endl;
    uint32_t retVal = skiq_write_rx_gain(driverdata->cardNum, tunerIndexHdl, gainIdx);
    if(retVal != 0){
        std::cout << "Error: Setting gain failed with error code " << retVal << std::endl;
    }
}

bool X40Driver_GetDcBias(X40DriverHandle handle, uint32_t tunerIndex) {
    if(handle == nullptr) {
        std::cout << "X40Driver_GetDcBias called with null handle" << std::endl;
        return false; // Invalid handle
    }
    std::cout << "X40Driver_GetDcBias called with tunerIndex: " << tunerIndex << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    bool isDCBiasEnabled;
    uint32_t retVal = skiq_read_rx_dc_offset_corr(driverdata->cardNum, tunerIndexHdl, &isDCBiasEnabled);
    if(retVal != 0){
        std::cout << "Error: Fetching DC Bias state failed with error code " << retVal << std::endl;
    }
    return isDCBiasEnabled;
}

void X40Driver_SetDcBias(X40DriverHandle handle, uint32_t tunerIndex, bool dcBias) {
    if(handle == nullptr) {
        std::cout << "X40Driver_SetDcBias called with null handle" << std::endl;
        return; // Invalid handle
    }
    std::cout << "X40Driver_SetDcBias called with tunerIndex: " << tunerIndex << " and dcBias: " << dcBias << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    uint32_t retVal = skiq_write_rx_dc_offset_corr(driverdata->cardNum, tunerIndexHdl, dcBias);
    if(retVal != 0){
        std::cout << "Error: Setting DC Bias state failed with error code " << retVal << std::endl;
    }
}

bool X40Driver_GetAgc(X40DriverHandle handle, uint32_t tunerIndex) {
    if(handle == nullptr) {
        std::cout << "X40Driver_GetAgc called with null handle" << std::endl;
        return false; // Invalid handle
    }
    std::cout << "X40Driver_GetAgc called with tunerIndex: " << tunerIndex << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    skiq_rx_gain_t gainMode; 
    int32_t retVal = skiq_read_rx_gain_mode (driverdata->cardNum, tunerIndexHdl, &gainMode);
    if(retVal != 0){
        std::cout << "Error: Fetching gain mode failed with error code " << retVal << std::endl;
    }
    return gainMode == skiq_rx_gain_auto;
}

void X40Driver_SetAgc(X40DriverHandle handle, uint32_t tunerIndex, bool agc) {
    if(handle == nullptr) {
        std::cout << "X40Driver_SetAgc called with null handle" << std::endl;
        return; // Invalid handle
    }
    std::cout << "X40Driver_SetAgc called with tunerIndex: " << tunerIndex << " and agc: " << agc << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    skiq_rx_gain_t gainMode = agc ? skiq_rx_gain_auto : skiq_rx_gain_manual; 
    int32_t retVal = skiq_write_rx_gain_mode (driverdata->cardNum, tunerIndexHdl, gainMode);
    if(retVal != 0){
        std::cout << "Error: Setting gain mode failed with error code " << retVal << std::endl;
    }
}

void X40Driver_StartRxStream(X40DriverHandle handle, uint32_t tunerIndex) {
    if(handle == nullptr) {
        std::cout << "X40Driver_StartRxStream called with null handle" << std::endl;
        return; // Invalid handle
    }
    if(tunerIndex >= X40_TUNER_COUNT) {
        std::cout << "X40Driver_StartRxStream called with invalid tuner index: " << tunerIndex << std::endl;
        return; // Invalid tuner index
    }
    // Check if the driver is in a valid state to start the RX stream
    std::cout << "X40Driver_StartRxStream called with tunerIndex: " << tunerIndex << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    if(!(driverdata->keepRunning && driverdata->keepReading)) {
        std::cout << "Error: invalid state (shutdown requested)" << std::endl;
        return; 
    }
    std::lock_guard<std::mutex> lock(driverdata->streamMutex);
    if(tunerIndex >= X40_TUNER_COUNT) {
        std::cout << "Error: Invalid tuner index " << tunerIndex << std::endl;
        return; // Invalid tuner index
    }
    if(driverdata->activeTuners[tunerIndex]) {
        std::cout << "Error: RX stream already active for tuner " << tunerIndex << std::endl;
        return; // Stream already active
    }
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);    
    uint32_t retVal = skiq_start_rx_streaming(driverdata->cardNum, tunerIndexHdl);
    if(retVal != 0){
        std::cout << "Error: Starting RX stream on tuner " << tunerIndex << " failed with error code " << retVal << std::endl;
        return; // Failed to start RX stream
    }
    
    // Start the RX processing thread for this tuner      
    driverdata->activeTuners[tunerIndex] = true; 
    std::cout << "Starting RX Processing Thread for tuner handle: " << tunerIndex << std::endl;    
    if(driverdata->tunerRxProcThreads[tunerIndex] == nullptr) {        
        std::thread *tunerRxProcThread = new std::thread(processRxDataForTuner, driverdata, tunerIndexHdl);          
        driverdata->tunerRxProcThreads[tunerIndex] = tunerRxProcThread;
    } else {
        std::cout << "RX Processing Thread already running for tuner handle: " << tunerIndex << std::endl;
    }

    // Check if the RX main read thread is already running, and if not, then start it
    if(!driverdata->isRxMainThreadActive) {
        std::cout << "Starting main RX read thread..." << std::endl;
        startMainRxReadThread(driverdata);
    }
}

void X40Driver_StopRxStream(X40DriverHandle handle, uint32_t tunerIndex) {
    if(handle == nullptr) {
        std::cout << "X40Driver_StopRxStream called with null handle" << std::endl;
        return; // Invalid handle
    }
    std::cout << "X40Driver_StopRxStream called with tunerIndex: " << tunerIndex << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    
    std::lock_guard<std::mutex> lock(driverdata->streamMutex);

    if(tunerIndex >= X40_TUNER_COUNT) {
        std::cout << "Error: Invalid tuner index " << tunerIndex << std::endl;
        return; // Invalid tuner index
    }
    if(!driverdata->activeTuners[tunerIndex]) {
        std::cout << "Error: RX stream not running for tuner " << tunerIndex << std::endl;
        return; // Stream already in active
    }
    
    uint32_t retVal = skiq_stop_rx_streaming(driverdata->cardNum, tunerIndexHdl);
    if(retVal != 0){
        std::cout << "Error: Stopping RX stream on tuner " << tunerIndex << " failed with error code " << retVal << std::endl;
    }
    driverdata->activeTuners[tunerIndex] = false; 
    std::thread *tunerRxProcThread = driverdata->tunerRxProcThreads[tunerIndex];
    if(tunerRxProcThread != nullptr) {        
        tunerRxProcThread->join();        
        delete tunerRxProcThread; // Clean up the thread object        
        driverdata->tunerRxProcThreads[tunerIndex] = nullptr;
    }    

}

float X40Driver_GetLatitude(X40DriverHandle handle) {
    if(handle == nullptr) {
        std::cout << "X40Driver_GetLatitude called with null handle" << std::endl;
        return 0.0f; // Invalid handle
    }
    std::cout << "X40Driver_GetLatitude called" << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    return driverdata->gpsInfo.latitude;
}

float X40Driver_GetLongitude(X40DriverHandle handle) {
    if(handle == nullptr) {
        std::cout << "X40Driver_GetLongitude called with null handle" << std::endl;
        return 0.0f; // Invalid handle
    }
    std::cout << "X40Driver_GetLongitude called" << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;  
    return driverdata->gpsInfo.longitude;
}

float X40Driver_GetElevation(X40DriverHandle handle) {
    if(handle == nullptr) {
        std::cout << "X40Driver_GetElevation called with null handle" << std::endl;
        return 0.0f; // Invalid handle
    }
    std::cout << "X40Driver_GetElevation called" << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;  
    return driverdata->gpsInfo.altitude;
}

uint32_t X40Driver_GetTxFrequency(X40DriverHandle handle, uint32_t tunerIndex) {
    if(handle == nullptr) {
        std::cout << "X40Driver_GetTxFrequency called with null handle" << std::endl;
        return 0; // Invalid handle
    }
    if(tunerIndex >= X40_TUNER_COUNT) {
        std::cout << "X40Driver_GetTxFrequency called with invalid tuner index: " << tunerIndex << std::endl;
        return 0; // Invalid tuner index
    }
    std::cout << "X40Driver_GetTxFrequency called with tunerIndex: " << tunerIndex << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    skiq_tx_hdl_t tunerIndexHdl = static_cast<skiq_tx_hdl_t>(tunerIndex);
    uint64_t programmedFreq;
    double actualFreq;
    int32_t retVal = skiq_read_tx_LO_freq(driverdata->cardNum, tunerIndexHdl, &programmedFreq, &actualFreq);
    return (uint32_t)actualFreq;
}

void X40Driver_SetTxFrequency(X40DriverHandle handle, uint32_t tunerIndex, uint32_t freq) {
    if(handle == nullptr) {
        std::cout << "X40Driver_SetTxFrequency called with null handle" << std::endl;
        return; // Invalid handle
    }
    if(tunerIndex >= X40_TUNER_COUNT) {
        std::cout << "X40Driver_SetTxFrequency called with invalid tuner index: " << tunerIndex << std::endl;
        return; // Invalid tuner index
    }
    std::cout << "X40Driver_SetTxFrequency called with tunerIndex: " << tunerIndex << " and freq: " << freq << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    skiq_tx_hdl_t tunerIndexHdl = static_cast<skiq_tx_hdl_t>(tunerIndex);
    uint32_t retVal = skiq_write_tx_LO_freq(driverdata->cardNum, tunerIndexHdl, (uint64_t) freq);
}

void X40Driver_StartTxStream(X40DriverHandle handle, uint32_t tunerIndex) {
    if(handle == nullptr) {
        std::cout << "X40Driver_StartTxStream called with null handle" << std::endl;
        return; // Invalid handle
    }
    if(tunerIndex >= X40_TUNER_COUNT) {
        std::cout << "X40Driver_StartTxStream called with invalid tuner index: " << tunerIndex << std::endl;
        return; // Invalid tuner index
    }
    std::cout << "X40Driver_StartTxStream called with tunerIndex: " << tunerIndex << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    skiq_tx_hdl_t tunerIndexHdl = static_cast<skiq_tx_hdl_t>(tunerIndex);
    uint32_t retVal = skiq_start_tx_streaming(driverdata->cardNum, tunerIndexHdl);
    if(retVal != 0){
        std::cout << "Error: Starting TX stream on tuner " << tunerIndex << " failed with error code " << retVal << std::endl;
    }
}

void X40Driver_StopTxStream(X40DriverHandle handle, uint32_t tunerIndex) {
    if(handle == nullptr) {
        std::cout << "X40Driver_StopTxStream called with null handle" << std::endl;
        return; // Invalid handle
    }
    if(tunerIndex >= X40_TUNER_COUNT) {
        std::cout << "X40Driver_StopTxStream called with invalid tuner index: " << tunerIndex << std::endl;
        return; // Invalid tuner index
    }
    std::cout << "X40Driver_StopTxStream called with tunerIndex: " << tunerIndex << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    skiq_tx_hdl_t tunerIndexHdl = static_cast<skiq_tx_hdl_t>(tunerIndex);
    uint32_t retVal = skiq_stop_tx_streaming(driverdata->cardNum, tunerIndexHdl);
    if(retVal != 0){
        std::cout << "Error: Stopping TX stream on tuner " << tunerIndex << " failed with error code " << retVal << std::endl;
    }
}

uint32_t X40Driver_GetPowerDbsSize(X40DriverHandle handle, uint32_t tunerIndex) {
    // X40DriverData *driverdata = (X40DriverData*)handle;
    // return driverdata->powerDbs[tunerIndex].size();
    return PKT_LEN/2; 
}
  
void X40Driver_GetPowerDbsData(X40DriverHandle handle, uint32_t tunerIndex, float* data, uint32_t size) {
    X40DriverData *driverdata = (X40DriverData*)handle;
    std::lock_guard<std::mutex> lock(driverdata->powerDbsMutexes[tunerIndex]);
    if(size >= driverdata->powerDbs[tunerIndex].size()){
        size = driverdata->powerDbs[tunerIndex].size();
    }    
    memcpy(data, driverdata->powerDbs[tunerIndex].data(), size * sizeof(float));    
}
//--------------------------------------------------------------------
// HELPER AND NON-API FUNCTION IMPLEMENTATIONS
//--------------------------------------------------------------------

/**
 * Gets the PROGRAMMED sample rate and bandwidth for the given tuner.
 */
uint32_t getProgrammedSRandBW(uint8_t cardNum, skiq_rx_hdl_t tunerHdl, uint32_t *sr, uint32_t *bw){    
    double actualSR;    
    uint32_t actualBW;
    int32_t retVal = skiq_read_rx_sample_rate_and_bandwidth(cardNum, tunerHdl, sr, &actualSR, bw, &actualBW);
    return retVal;
}

/**
 * Gets the ACTUAL sample rate and bandwidth for the given tuner.
 */
uint32_t getActualSRandBW(uint8_t cardNum, skiq_rx_hdl_t tunerHdl, double *sr, uint32_t *bw){    
    uint32_t progSR;    
    uint32_t progBW;
    int32_t retVal = skiq_read_rx_sample_rate_and_bandwidth(cardNum, tunerHdl, &progSR, sr, &progBW, bw);
    return retVal;
}

void getGainRangeSettings(uint8_t cardNum, skiq_rx_hdl_t tunerHdl){
    uint8_t gainIdxMin, gainIdxMax; 
    int32_t retVal = skiq_read_rx_gain_index_range (cardNum, tunerHdl, &gainIdxMin, &gainIdxMax);
    std::cout << "Retval: " << retVal << std::endl;
    std::cout << "Min index: " << static_cast<uint32_t>(gainIdxMin) << "; Max index: " << static_cast<uint32_t>(gainIdxMax) << "; Step: 1 idx = 0.5dB" << std::endl;
}

uint8_t getGainIdxFromGainDB(uint32_t gainValDB, uint8_t idxMin, uint8_t idxMax){
    /**
     * y = m*x + b; m = (Change in index) / (Change in gainValDB)
     *   m = (idx_max - idxMin) / (DB_GAIN_MAX - DB_GAIN_MIN)
     * gainValDB = 0 -> idx = idxMin so b = idxMin 
    */
   uint8_t gainValClipped = gainValDB > DB_GAIN_MAX ? DB_GAIN_MAX : static_cast<uint8_t>(gainValDB);
   float scaleFac = (idxMax - idxMin) / DB_GAIN_MAX; //DB_GAIN_MIN is 0
   uint8_t scaleFacInt = static_cast<uint8_t>(scaleFac);
   uint8_t idx = (scaleFacInt * gainValClipped) + idxMin;
   return idx;
}

uint32_t getGainDBFromGainIdx(uint8_t idx, uint8_t idxMin, uint8_t idxMax){
    /**
     * Using the math from the inverse function of this....
     * x = (y - b) / m 
    */
   uint8_t idxClipped = idx > idxMax ? idxMax : idx;
   float gainDB = (idxClipped - idxMin) * (DB_GAIN_MAX/(idxMax - idxMin));
   return static_cast<uint32_t>(gainDB);
}

void startGPSPolling(X40DriverData *driverdata){
    if(driverdata == nullptr) {
        std::cout << "Error: driverdata is null in startGPSPolling" << std::endl;
        return;
    }
    if(driverdata->gpsThread == nullptr){
        std::cout << "Starting GPS Polling Thread" << std::endl;
        std::thread *gpsThread = new std::thread(gpsPollLoop, driverdata->zynqIP, driverdata->zynqGPSDPort, 
            std::cref(driverdata->keepRunning), &(driverdata->gpsInfo), USECONDS_IN_SECOND/2);
        driverdata->gpsThread = gpsThread;
    }
}

void processRxDataForTuner(X40DriverData *driverdata, skiq_rx_hdl_t tunerHdl){
    if(driverdata == nullptr) {
        std::cout << "Error: driverdata is null in processRxDataForTuner" << std::endl;
        return;
    }
    if(tunerHdl >= X40_TUNER_COUNT) {
        std::cout << "Error: Invalid tuner handle in processRxDataForTuner: " << tunerHdl << std::endl;
        return; // Invalid tuner handle
    }
    std::cout << "Processing RX data for tuner handle: " << tunerHdl << std::endl;
    // metrics/testing
    int readCount = 0;
    // Allocate resources for performance
    std::array<int16_t, PKT_LEN> pktData = {0};    
    bool bufReadRes = false;

    // Resources for interleaved short-to-complex float conversion
    int complexFloatBatchSize = PKT_LEN/2;
    size_t shortTofloatInputSizeBytes = PKT_LEN * sizeof(short);
    size_t shortToFloatOutputSizeBytes = complexFloatBatchSize * sizeof(float2);

    short* d_input = nullptr;
    float2* d_output = nullptr;
    CUDA_CHECK(cudaMalloc((void**)&d_input, shortTofloatInputSizeBytes));
    CUDA_CHECK(cudaMalloc((void**)&d_output, shortToFloatOutputSizeBytes));

    int threadsPerBlock = 128; // Threads per block (multiple of 32)    
    int blocksPerGrid = (complexFloatBatchSize + threadsPerBlock - 1) / threadsPerBlock;
   
    // Resources for FFT    
    float2* d_fftOutput = nullptr;
    size_t fftBufferSizeBytes = sizeof(float2) * complexFloatBatchSize;
    CUDA_CHECK(cudaMalloc((void**)&d_fftOutput, fftBufferSizeBytes));

    // Resources for PowerDB calculation
    float* d_powerDb = nullptr;
    float* d_powerDbShifted = nullptr;
    size_t powerBufferSizeBytes = sizeof(float) * complexFloatBatchSize;
    CUDA_CHECK(cudaMalloc((void**)&d_powerDb, powerBufferSizeBytes));
    CUDA_CHECK(cudaMalloc((void**)&d_powerDbShifted, powerBufferSizeBytes));

    const float epsilon = 1e-12f; // Small value to avoid log10(0)
    const float dbFloor = -150.0f; // Minimum dB value

    cufftHandle plan;
    std::cout << "Creating cuFFT plan (1D C2C)..." << std::endl;
    CUFFT_CHECK(cufftPlan1d(&plan, complexFloatBatchSize, CUFFT_C2C, 1));
    std::cout << "cuFFT plan created." << std::endl;

    // Process the data for this tuner
    X40DataBuffer *buffer = driverdata->rxBuffers[tunerHdl];
    if(buffer == nullptr) {
        std::cout << "Error: No buffer found for tuner handle: " << tunerHdl << std::endl;
        return;
    }
    while(driverdata->keepRunning && driverdata->activeTuners[tunerHdl]){ 
        // bufReadRes = true; //
        bufReadRes = buffer->read(pktData);
        // std::fill(pktData.begin(), pktData.end(), readCount);
        if(!bufReadRes) {
            std::this_thread::sleep_for(std::chrono::microseconds(50)); // Sleep to avoid busy-waiting
            continue; // No data to process, skip this iteration
        }
        readCount++;
        // Convert short I/Q data to complex float format
        CUDA_CHECK(cudaMemcpy(d_input, pktData.data(), shortTofloatInputSizeBytes, cudaMemcpyHostToDevice));
        interleavedShortToComplexFloat<<<blocksPerGrid, threadsPerBlock>>>(d_input, d_output, complexFloatBatchSize);
        CUDA_CHECK(cudaGetLastError());
        CUDA_CHECK(cudaDeviceSynchronize());

        // Perform FFT on the complex float data
        CUFFT_CHECK(cufftExecC2C(plan, d_output, d_fftOutput, CUFFT_FORWARD));
        CUDA_CHECK(cudaGetLastError());
        CUDA_CHECK(cudaDeviceSynchronize());

        // Calculate the power in dB
        calculateLogPowerKernel<<<blocksPerGrid, threadsPerBlock>>>(
            d_fftOutput, d_powerDb, complexFloatBatchSize, epsilon, dbFloor
        );
        CUDA_CHECK(cudaDeviceSynchronize());

        // Shift the FFT output to center the zero frequency component
        fftShiftKernel<<<blocksPerGrid, threadsPerBlock>>>(
            d_powerDb, d_powerDbShifted, complexFloatBatchSize
        );
        CUDA_CHECK(cudaGetLastError());
        // Synchronize needed before copying result back to host
        CUDA_CHECK(cudaDeviceSynchronize());

        std::lock_guard<std::mutex> lock(driverdata->powerDbsMutexes[tunerHdl]);
        CUDA_CHECK(cudaMemcpy(driverdata->powerDbs[tunerHdl].data(), d_powerDbShifted, powerBufferSizeBytes, cudaMemcpyDeviceToHost));
        std::this_thread::sleep_for(std::chrono::microseconds(10)); // Sleep to avoid busy-waiting
    }
    // --- Cleanup ---
    std::cout << "Destroying cuFFT plan..." << std::endl;
    CUFFT_CHECK(cufftDestroy(plan));
    std::cout << "cuFFT plan destroyed." << std::endl;

    std::cout << "Freeing GPU (device) memory..." << std::endl;
    CUDA_CHECK(cudaFree(d_input));
    CUDA_CHECK(cudaFree(d_output));
    CUDA_CHECK(cudaFree(d_fftOutput));
    CUDA_CHECK(cudaFree(d_powerDb));
    CUDA_CHECK(cudaFree(d_powerDbShifted));
    std::cout << "Device memory freed." << std::endl;
    std::cout << "Processed " << readCount << " packets for tuner handle: " << tunerHdl << std::endl;
    std::cout << "RX Processing Thread for tuner handle " << tunerHdl << " exiting." << std::endl;
}

void readRxData(X40DriverData *driverdata){
    std::cout << "ENTERING readRxData" << std::endl;
    if(driverdata == nullptr) {
        std::cout << "Error: driverdata is null in readRxData" << std::endl;
        return;
    }
    skiq_rx_hdl_t hdl;
    uint32_t rcvLenBytes;

    uint64_t nextTs;
    uint64_t currTS;
    bool firstBlock = true;
    uint32_t totalReadCount = 0;
    uint32_t droppedDataInstanceCount = 0;
    uint32_t readTooFastInstanceCount = 0;
    
    skiq_rx_block_t *rxBlk;
    int writeStatus;
    int writeFails = 0;

    uint32_t dataLen;
    uint32_t recvSampleCount;
    // std::srand(std::time(0));
    // std::array<int16_t, PKT_LEN> DUMMY_pktData;
    while(driverdata->keepRunning && driverdata->keepReading){
        // for (int i = 0; i < PKT_LEN; ++i) {
        //     DUMMY_pktData[i] = (std::rand() % 12799) + 1;
        // }
        // writeStatus = driverdata->rxBuffers[0]->insert(DUMMY_pktData.data());
        // if(!writeStatus){
        //     writeFails++;
        // }
        // totalReadCount++;
        // std::this_thread::sleep_for(std::chrono::microseconds(500));
        int32_t rx_status = skiq_receive(driverdata->cardNum, &hdl, &rxBlk, &rcvLenBytes);
        if(rx_status == 0){
            totalReadCount++;
            dataLen = (rcvLenBytes/4) - SKIQ_RX_HEADER_SIZE_IN_WORDS;
            currTS = rxBlk->rf_timestamp;
            if(firstBlock == true){
                firstBlock = false;
                nextTs = currTS;
                nextTs += dataLen;
            }
            else{
                if(currTS != nextTs){
                    // std::cout << "WARN: timestamp diff! " << (currTS - nextTs) << std::endl;
                    if(nextTs < currTS){
                        droppedDataInstanceCount++;
                    }else{
                        readTooFastInstanceCount++;
                    }
                    nextTs = currTS;
                    nextTs += dataLen;                    
                }
                else{
                    nextTs += dataLen;
                }
            }
            //Add the data to the buffer corresponding to the tuner handle
            if(!driverdata->activeTuners[hdl]){
                std::cout << "WARN: Received data for inactive tuner handle: " << hdl << std::endl;
                continue; // Skip processing for inactive tuners
            }
            recvSampleCount = dataLen*2;
            // if(totalReadCount %5000 == 0){
            //     std::cout << "dataLen = " << dataLen << "; recvSampleCount " << recvSampleCount << std::endl;
            // }
            writeStatus = driverdata->rxBuffers[hdl]->insert((const int16_t *)rxBlk->data, recvSampleCount);
            if(!writeStatus){
                writeFails++;
            }
        }
    }
    std::cout << "*** SUMMARY:"<< std::endl;
    std::cout << "*** Total Reads: " << totalReadCount << " reads " << std::endl;
    std::cout << "*** Missed data on " << droppedDataInstanceCount << " reads " << std::endl;
    std::cout << "*** Read too fast on " << readTooFastInstanceCount << " reads " << std::endl;
    std::cout << "*** Failed to write to buffer " << writeFails << " times " << std::endl;

}

void startMainRxReadThread(X40DriverData *driverdata){
    if(driverdata == nullptr) {
        std::cout << "Error: driverdata is null in startMainRxReadThread" << std::endl;
        return;
    }
    if(driverdata->rxMainThread == nullptr && !driverdata->isRxMainThreadActive){
        std::cout << "Starting RX Main Read Thread..." << std::endl;
        std::thread *rxMainThread = new std::thread(readRxData, driverdata);
        driverdata->rxMainThread = rxMainThread;
        driverdata->isRxMainThreadActive = true;
    } else {
        std::cout << "RX Main Read Thread already running!" << std::endl;
    }
}

void stopMainRxReadThread(X40DriverHandle handle){
    if(handle == nullptr) {
        std::cout << "Error: handle is null in stopMainRxReadThread" << std::endl;
        return;
    }
    std::cout << "Stopping RX Main Read Thread..." << std::endl;
    X40DriverData *driverdata = (X40DriverData*)handle;
    std::thread *rxMainThread = driverdata->rxMainThread;
    if(rxMainThread != nullptr) {
        driverdata->keepReading = false; 
        rxMainThread->join();        
        delete rxMainThread;        
        driverdata->rxMainThread = nullptr;
        driverdata->isRxMainThreadActive = false;
    }
    std::cout << "RX Main Read Thread Stopped" << std::endl;
}

void stopRxProcessingThreads(X40DriverHandle handle){
    if(handle == nullptr) {
        std::cout << "Error: handle is null in stopRxProcessingThreads" << std::endl;
        return;
    }
    std::cout << "Stopping RX Processing Threads..." << std::endl;    
    for(int i = 0; i < X40_TUNER_COUNT; ++i) {
        X40Driver_StopRxStream(handle, i);        
    }
    std::cout << "RX Processing Threads Stopped" << std::endl; 
}

void stopAllThreads(X40DriverHandle handle) {
    if(handle == nullptr) {
        std::cout << "Error: handle is null in stopAllThreads" << std::endl;
        return;
    }
    std::cout << "Stopping all threads..." << std::endl;
    stopMainRxReadThread(handle);
    stopRxProcessingThreads(handle);
    X40DriverData *driverdata = (X40DriverData*)handle;
    driverdata->keepRunning = false;
    if(driverdata->gpsThread != nullptr) {
        std::cout << "Stopping GPS thread" << std::endl;       
        driverdata->gpsThread->join();          
        delete driverdata->gpsThread;        
        driverdata->gpsThread = nullptr;
    }    
}

void skiqSafeRestartRxStream(X40DriverData *driverdata, skiq_rx_hdl_t tunerHdl){
    std::lock_guard<std::mutex> lock(driverdata->streamMutex);
    if(driverdata->activeTuners[tunerHdl]){
        skiq_stop_rx_streaming(driverdata->cardNum, tunerHdl);
        std::this_thread::sleep_for(std::chrono::milliseconds(1));
        skiq_start_rx_streaming(driverdata->cardNum, tunerHdl);
    }
}


#include "b210_driver.h"
#include <iostream>
#include <uhd/utils/thread.hpp>
#include <uhd/utils/safe_main.hpp>
#include <uhd/usrp/multi_usrp.hpp>
#include <uhd/exception.hpp>
#include <boost/interprocess/shared_memory_object.hpp>
#include <boost/interprocess/mapped_region.hpp>
#include <boost/interprocess/sync/named_semaphore.hpp>
#include <stdint.h>
#include "nmea.h"
#include <thread>
#include <mutex>

// CUDA includes
#include <cuda_runtime.h>
#include <cufft.h> // Main cuFFT header

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

const int STOP_TIME = 100; // Time in milliseconds to wait for the stream to stop

namespace bip = boost::interprocess;

struct B210DriverData {
  uhd::usrp::multi_usrp::sptr usrp;
  uhd::rx_streamer::sptr rx_stream[2];
  std::thread *rx_thread = nullptr;
  bool stop_signal = false;
  std::mutex fft_mutex;
  std::vector<std::vector<float>> power_dbs;
  bool coherent_mode = false;
};

B210DriverHandle B210Driver_Init(const char* args) {
    B210DriverData* data = new B210DriverData();
    try {
        data->usrp = uhd::usrp::multi_usrp::make(args);
    } catch (uhd::exception& e) {
        std::cerr << "B210 Error: " << e.what() << std::endl;
        delete data;
        return nullptr;
    }
    return (B210DriverHandle)data;
}

uint32_t B210Driver_GetNumTuners(B210DriverHandle handle) {
  B210DriverData* data = (B210DriverData*)handle;
  return data->usrp->get_rx_num_channels();
}

uint32_t B210Driver_GetRxFrequency(B210DriverHandle handle, uint32_t tunerIndex) {
  B210DriverData* data = (B210DriverData*)handle;
  return (uint32_t)data->usrp->get_rx_freq(tunerIndex);
}

void B210Driver_SetRxFrequency(B210DriverHandle handle, uint32_t tunerIndex, uint32_t freq) {
  B210DriverData* data = (B210DriverData*)handle;
  // B210Driver_StopRxStream(handle, tunerIndex); // Stop the stream before changing frequency
  // std::this_thread::sleep_for(std::chrono::milliseconds(STOP_TIME)); // Give some time for the stream to stop
  data->usrp->set_rx_freq(freq, tunerIndex);
  // B210Driver_StartRxStream(handle, tunerIndex); // Restart the stream after changing frequency
}

uint32_t B210Driver_GetSampleRate(B210DriverHandle handle, uint32_t tunerIndex) {
  B210DriverData* data = (B210DriverData*)handle;
  return (uint32_t)data->usrp->get_rx_rate(tunerIndex);
}

void B210Driver_SetSampleRate(B210DriverHandle handle, uint32_t tunerIndex, uint32_t rate) {
  B210DriverData* data = (B210DriverData*)handle;
  // B210Driver_StopRxStream(handle, tunerIndex); // Stop the stream before changing rate
  // std::this_thread::sleep_for(std::chrono::milliseconds(STOP_TIME)); // Give some time for the stream to stop
  data->usrp->set_rx_rate(rate, tunerIndex);
  // B210Driver_StartRxStream(handle, tunerIndex); // Restart the stream after changing rate
}

uint32_t B210Driver_GetRxBandwidth(B210DriverHandle handle, uint32_t tunerIndex) {
  B210DriverData* data = (B210DriverData*)handle;
  return (uint32_t)data->usrp->get_rx_bandwidth(tunerIndex);
}

void B210Driver_SetRxBandwidth(B210DriverHandle handle, uint32_t tunerIndex, uint32_t bandwidth) {
  B210DriverData* data = (B210DriverData*)handle;
  // B210Driver_StopRxStream(handle, tunerIndex); // Stop the stream before changing bandwidth
  // std::this_thread::sleep_for(std::chrono::milliseconds(STOP_TIME)); // Give some time for the stream to stop
  data->usrp->set_rx_bandwidth(bandwidth, tunerIndex);
  // B210Driver_StartRxStream(handle, tunerIndex); // Restart the stream after changing bandwidth
}

uint32_t B210Driver_GetGain(B210DriverHandle handle, uint32_t tunerIndex) {
  B210DriverData* data = (B210DriverData*)handle;
  return (uint32_t)data->usrp->get_rx_gain(tunerIndex);
}

void B210Driver_SetGain(B210DriverHandle handle, uint32_t tunerIndex, uint32_t gain) {
  B210DriverData* data = (B210DriverData*)handle;
  data->usrp->set_rx_gain(gain, tunerIndex);
}

bool B210Driver_GetDcBias(B210DriverHandle handle, uint32_t tunerIndex) {
  B210DriverData* data = (B210DriverData*)handle;
  //Implement DC bias getter
  return false;
}

void B210Driver_SetDcBias(B210DriverHandle handle, uint32_t tunerIndex, bool dcBias) {
   B210DriverData* data = (B210DriverData*)handle;
  //Implement DC bias setter
}

bool B210Driver_GetAgc(B210DriverHandle handle, uint32_t tunerIndex) {
  B210DriverData* data = (B210DriverData*)handle;
  //Implement AGC getter
  return false;
}

void B210Driver_SetAgc(B210DriverHandle handle, uint32_t tunerIndex, bool agc) {
  B210DriverData* data = (B210DriverData*)handle;
  //Implement AGC setter
}

void B210Driver_StartRxStream(B210DriverHandle handle, uint32_t tunerIndex) {
  B210DriverData* data = (B210DriverData*)handle;

  uhd::stream_args_t stream_args0("fc32");
  uhd::stream_args_t stream_args1("fc32");

  // Setup stream 0
  stream_args0.channels = {0};
  data->rx_stream[0] = data->usrp->get_rx_stream(stream_args0);
  uhd::stream_cmd_t stream_cmd0(uhd::stream_cmd_t::STREAM_MODE_START_CONTINUOUS);
  stream_cmd0.num_samps = 0;
  data->rx_stream[0]->issue_stream_cmd(stream_cmd0);

  // Setup stream 1
  stream_args1.channels = {1};
  data->rx_stream[1] = data->usrp->get_rx_stream(stream_args1);
  uhd::stream_cmd_t stream_cmd1(uhd::stream_cmd_t::STREAM_MODE_START_CONTINUOUS);
  stream_cmd1.num_samps = 0;
  data->rx_stream[1]->issue_stream_cmd(stream_cmd1);

  // Launch the receive thread
  data->stop_signal = false;
  data->rx_thread = new std::thread(&B210Driver_ReceiveLoop, handle);
}

void B210Driver_StopRxStream(B210DriverHandle handle, uint32_t tunerIndex) {
  B210DriverData* data = (B210DriverData*)handle;
  data->stop_signal = true;
  std::this_thread::sleep_for(std::chrono::milliseconds(STOP_TIME)); // Give some time for the thread to stop
  if (data->rx_thread && data->rx_thread->joinable()) {
    data->rx_thread->join();
    delete data->rx_thread;
    data->rx_thread = nullptr;
    data->stop_signal = false;
  }

  data->rx_stream[0]->issue_stream_cmd(uhd::stream_cmd_t::STREAM_MODE_STOP_CONTINUOUS);
  data->rx_stream[0] = nullptr;
  data->rx_stream[1]->issue_stream_cmd(uhd::stream_cmd_t::STREAM_MODE_STOP_CONTINUOUS);
  data->rx_stream[1] = nullptr;
}

uint32_t B210Driver_GetTxFrequency(B210DriverHandle handle, uint32_t tunerIndex) {
  B210DriverData* data = (B210DriverData*)handle;
    return (uint32_t)data->usrp->get_tx_freq(tunerIndex);
}

void B210Driver_SetTxFrequency(B210DriverHandle handle, uint32_t tunerIndex, uint32_t freq) {
  B210DriverData* data = (B210DriverData*)handle;
   data->usrp->set_tx_freq(freq, tunerIndex);
}

void B210Driver_StartTxStream(B210DriverHandle handle, uint32_t tunerIndex) {
  B210DriverData* data = (B210DriverData*)handle;
  //Implement Start TX Stream
}

void B210Driver_StopTxStream(B210DriverHandle handle, uint32_t tunerIndex) {
  B210DriverData* data = (B210DriverData*)handle;
  //Implement Stop TX Stream
}

void B210Driver_Close(B210DriverHandle handle) {
    B210DriverData* data = (B210DriverData*)handle;
    delete data;
}

float B210Driver_GetLatitude(B210DriverHandle handle) {
  B210DriverData* data = (B210DriverData*)handle;
  // make sure the GPSDO really has lock, otherwise you'll only get zeros
  if (!data->usrp->get_mboard_sensor("gps_locked").to_bool())
    return 0.0f;

  const auto gga = data->usrp->get_mboard_sensor("gps_gpgga");
  // `value` holds the raw NMEA sentence without the pretty‑print prefix
  auto gps = parse_gpgga(gga.value);
  return gps.lat;
}

float B210Driver_GetLongitude(B210DriverHandle handle) {
  B210DriverData* data = (B210DriverData*)handle;
  // make sure the GPSDO really has lock, otherwise you'll only get zeros
  if (!data->usrp->get_mboard_sensor("gps_locked").to_bool())
    return 0.0f;

  const auto gga = data->usrp->get_mboard_sensor("gps_gpgga");
  // `value` holds the raw NMEA sentence without the pretty‑print prefix
  auto gps = parse_gpgga(gga.value);
  return gps.lon;
}

float B210Driver_GetElevation(B210DriverHandle handle) {
  B210DriverData* data = (B210DriverData*)handle;
  // make sure the GPSDO really has lock, otherwise you'll only get zeros
  if (!data->usrp->get_mboard_sensor("gps_locked").to_bool())
    return 0.0f;

  const auto gga = data->usrp->get_mboard_sensor("gps_gpgga");
  // `value` holds the raw NMEA sentence without the pretty‑print prefix
  auto gps = parse_gpgga(gga.value);
  return gps.alt;
}

void B210Driver_ReceiveLoop(B210DriverHandle handle) {
  B210DriverData* data = (B210DriverData*)handle;
  uhd::rx_metadata_t md;
  size_t samps_per_buff = 0;
  const size_t num_channels = 2;
  size_t stream0_max = data->rx_stream[0]->get_max_num_samps();
  size_t stream1_max = data->rx_stream[1]->get_max_num_samps();
  samps_per_buff = stream0_max > stream1_max ? stream0_max : stream1_max;

  std::vector<std::vector<std::complex<float>>> buffs(2, std::vector<std::complex<float>>(samps_per_buff));
  data->power_dbs.resize(num_channels, std::vector<float>(samps_per_buff));

  std::cout << "Coherent Mode: " << data->coherent_mode << std::endl;
  std::cout << "Samps_per_buff = " << samps_per_buff << std::endl;

  std::vector<std::complex<float>*> buff_ptrs;
  for (size_t i = 0; i < buffs.size(); i++)
    buff_ptrs.push_back(&buffs[i].front());

  cufftComplex *d_input_data = nullptr;
  cufftComplex *d_output_data = nullptr;
  size_t buffer_size_bytes = sizeof(cufftComplex) * samps_per_buff;

  std::cout << "Allocating GPU (device) memory (" << buffer_size_bytes << " bytes each)..." << std::endl;
  CUDA_CHECK(cudaMalloc((void**)&d_input_data, buffer_size_bytes));
  CUDA_CHECK(cudaMalloc((void**)&d_output_data, buffer_size_bytes));
  std::cout << "Device memory allocated." << std::endl;

  float* d_power_db = nullptr;
  float* d_power_db_shifted = nullptr;
  size_t power_buffer_size_bytes = sizeof(float) * samps_per_buff;
  CUDA_CHECK(cudaMalloc((void**)&d_power_db, power_buffer_size_bytes));
  CUDA_CHECK(cudaMalloc((void**)&d_power_db_shifted, power_buffer_size_bytes));

  const float epsilon = 1e-12f; // Small value to avoid log10(0)
  const float db_floor = -150.0f; // Minimum dB value

  cufftHandle plan;
  std::cout << "Creating cuFFT plan (1D C2C)..." << std::endl;
  CUFFT_CHECK(cufftPlan1d(&plan, samps_per_buff, CUFFT_C2C, 1));
  std::cout << "cuFFT plan created." << std::endl;

  while (!data->stop_signal) {
    size_t num_rx_samps[2];

    num_rx_samps[0] = data->rx_stream[0]->recv(buff_ptrs[0], samps_per_buff, md, 1.0);
    if (md.error_code != uhd::rx_metadata_t::ERROR_CODE_NONE) {
      std::cerr << "Tuner 0 Receive error: " << md.strerror() << std::endl;
      continue;
    }

    num_rx_samps[1] = data->rx_stream[1]->recv(buff_ptrs[1], samps_per_buff, md, 1.0);
    if (md.error_code != uhd::rx_metadata_t::ERROR_CODE_NONE) {
      std::cerr << "Tuner 1 Receive error: " << md.strerror() << std::endl;
      continue;
    }
  
    for(size_t i = 0; i < num_channels; ++i) {
      size_t fft_size = num_rx_samps[i];
      CUDA_CHECK(cudaMemcpy(d_input_data, buff_ptrs[i], buffer_size_bytes, cudaMemcpyHostToDevice));
      CUFFT_CHECK(cufftExecC2C(plan, d_input_data, d_output_data, CUFFT_FORWARD));
      CUDA_CHECK(cudaGetLastError());

      int threadsPerBlock = 256;
      int blocksPerGrid = (fft_size + threadsPerBlock - 1) / threadsPerBlock;
      calculateLogPowerKernel<<<blocksPerGrid, threadsPerBlock>>>(
          d_output_data, d_power_db, fft_size, epsilon, db_floor
      );
      CUDA_CHECK(cudaGetLastError()); // Check for kernel launch errors
      // Optional: Synchronize if subsequent kernel depends *immediately* on this one,
      // but often not needed if just launching another kernel or doing D->H copy.
      CUDA_CHECK(cudaDeviceSynchronize());

      // **Step 5: Execute FFT Shift Kernel on GPU**
      // Uses d_power_db -> d_power_db_shifted
      fftShiftKernel<<<blocksPerGrid, threadsPerBlock>>>(
          d_power_db, d_power_db_shifted, fft_size
      );
      CUDA_CHECK(cudaGetLastError());
      // Synchronize needed before copying result back to host
      CUDA_CHECK(cudaDeviceSynchronize());

      // **Step 6: Copy final result (Shifted dB Power) Device -> Host**
      // Assuming data->ffts[i] is now std::vector<float> or similar
      data->fft_mutex.lock(); // Protect shared host buffer access
      // Ensure data->ffts[i] is sized correctly (fft_size)
      if (data->power_dbs[i].size() != fft_size) {
          data->power_dbs[i].resize(fft_size); // Resize if necessary
      }
      CUDA_CHECK(cudaMemcpy(data->power_dbs[i].data(), d_power_db_shifted, power_buffer_size_bytes, cudaMemcpyDeviceToHost));
      data->fft_mutex.unlock();
    }
  }

  // --- Cleanup ---
  std::cout << "Destroying cuFFT plan..." << std::endl;
  CUFFT_CHECK(cufftDestroy(plan));
  std::cout << "cuFFT plan destroyed." << std::endl;

  std::cout << "Freeing GPU (device) memory..." << std::endl;
  CUDA_CHECK(cudaFree(d_input_data));
  CUDA_CHECK(cudaFree(d_output_data));
  std::cout << "Device memory freed." << std::endl;
}

uint32_t B210Driver_GetPowerDbsSize(B210DriverHandle handle, uint32_t tunerIndex) {
  B210DriverData* data = (B210DriverData*)handle;
  return data->power_dbs[tunerIndex].size();
}

void B210Driver_GetPowerDbsData(B210DriverHandle handle, uint32_t tunerIndex, float* data, uint32_t size) {
  B210DriverData* driver = (B210DriverData*)handle;
  driver->fft_mutex.lock();
  memcpy(data, driver->power_dbs[tunerIndex].data(), size * sizeof(float));
  driver->fft_mutex.unlock();
}



#include "x310_driver.h"
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

struct X310DriverData {
  uhd::usrp::multi_usrp::sptr usrp;
  const size_t num_rx_channels;
  const size_t num_tx_channels;
  uhd::rx_streamer::sptr *rx_stream;
  std::thread *rx_thread = nullptr;
  bool stop_signal = false;
  std::mutex *fft_mutex;
  std::vector<std::vector<float>> power_dbs;
  bool coherent_mode = false;

  X310DriverData(size_t num_rx, size_t num_tx) : num_rx_channels(num_rx), num_tx_channels(num_tx) {
      rx_stream = new uhd::rx_streamer::sptr[num_rx_channels];
      fft_mutex = new std::mutex[num_rx_channels];
  }
  X310DriverData() : num_rx_channels(2), num_tx_channels(0) {
      rx_stream = new uhd::rx_streamer::sptr[num_rx_channels];
      fft_mutex = new std::mutex[num_rx_channels];
  }
  ~X310DriverData() {
      delete[] rx_stream;
      delete[] fft_mutex;
  }
};

// TODO generic to Ettus USRP
X310DriverHandle X310Driver_Init(const char* args) {
    uhd::usrp::multi_usrp::sptr new_usrp;
    X310DriverData* data;
    try {
        new_usrp = uhd::usrp::multi_usrp::make(std::string(args));
    } catch (uhd::exception& e) {
        std::cerr << "X310 Error: " << e.what() << std::endl;
        return nullptr;
    }
    data = new X310DriverData(new_usrp->get_rx_num_channels(), 0);
    data->usrp = new_usrp;
    return (X310DriverHandle)data;
}

uint32_t X310Driver_GetNumTuners(X310DriverHandle handle) {
  X310DriverData* data = (X310DriverData*)handle;
  return data->usrp->get_rx_num_channels();
}

uint32_t X310Driver_GetRxFrequency(X310DriverHandle handle, uint32_t tunerIndex) {
  X310DriverData* data = (X310DriverData*)handle;
  return (uint32_t)data->usrp->get_rx_freq(tunerIndex);
}

void X310Driver_SetRxFrequency(X310DriverHandle handle, uint32_t tunerIndex, uint32_t freq) {
  X310DriverData* data = (X310DriverData*)handle;
  // X310Driver_StopRxStream(handle, tunerIndex); // Stop the stream before changing frequency
  // std::this_thread::sleep_for(std::chrono::milliseconds(STOP_TIME)); // Give some time for the stream to stop
  data->usrp->set_rx_freq(freq, tunerIndex);
  // X310Driver_StartRxStream(handle, tunerIndex); // Restart the stream after changing frequency
}

uint32_t X310Driver_GetSampleRate(X310DriverHandle handle, uint32_t tunerIndex) {
  X310DriverData* data = (X310DriverData*)handle;
  return (uint32_t)data->usrp->get_rx_rate(tunerIndex);
}

void X310Driver_SetSampleRate(X310DriverHandle handle, uint32_t tunerIndex, uint32_t rate) {
  X310DriverData* data = (X310DriverData*)handle;
  // X310Driver_StopRxStream(handle, tunerIndex); // Stop the stream before changing rate
  // std::this_thread::sleep_for(std::chrono::milliseconds(STOP_TIME)); // Give some time for the stream to stop
  data->usrp->set_rx_rate(rate, tunerIndex);
  // X310Driver_StartRxStream(handle, tunerIndex); // Restart the stream after changing rate
}

uint32_t X310Driver_GetRxBandwidth(X310DriverHandle handle, uint32_t tunerIndex) {
  X310DriverData* data = (X310DriverData*)handle;
  return (uint32_t)data->usrp->get_rx_bandwidth(tunerIndex);
}

void X310Driver_SetRxBandwidth(X310DriverHandle handle, uint32_t tunerIndex, uint32_t bandwidth) {
  X310DriverData* data = (X310DriverData*)handle;
  // X310Driver_StopRxStream(handle, tunerIndex); // Stop the stream before changing bandwidth
  // std::this_thread::sleep_for(std::chrono::milliseconds(STOP_TIME)); // Give some time for the stream to stop
  data->usrp->set_rx_bandwidth(bandwidth, tunerIndex);
  // X310Driver_StartRxStream(handle, tunerIndex); // Restart the stream after changing bandwidth
}

uint32_t X310Driver_GetGain(X310DriverHandle handle, uint32_t tunerIndex) {
  X310DriverData* data = (X310DriverData*)handle;
  return (uint32_t)data->usrp->get_rx_gain(tunerIndex);
}

void X310Driver_SetGain(X310DriverHandle handle, uint32_t tunerIndex, uint32_t gain) {
  X310DriverData* data = (X310DriverData*)handle;
  data->usrp->set_rx_gain(gain, tunerIndex);
}

bool X310Driver_GetDcBias(X310DriverHandle handle, uint32_t tunerIndex) {
  X310DriverData* data = (X310DriverData*)handle;
  // TODO Implement DC bias getter
  return false;
}

void X310Driver_SetDcBias(X310DriverHandle handle, uint32_t tunerIndex, bool dcBias) {
   X310DriverData* data = (X310DriverData*)handle;
  // TODO Implement DC bias setter
}

bool X310Driver_GetAgc(X310DriverHandle handle, uint32_t tunerIndex) {
  X310DriverData* data = (X310DriverData*)handle;
  // TODO Implement AGC getter
  return false;
}

void X310Driver_SetAgc(X310DriverHandle handle, uint32_t tunerIndex, bool agc) {
  X310DriverData* data = (X310DriverData*)handle;
  // TODO Implement AGC setter
}

void X310Driver_StartRxStream(X310DriverHandle handle, uint32_t tunerIndex) {
  X310DriverData* data = (X310DriverData*)handle;

  std::vector<uhd::stream_args_t> stream_args(data->num_rx_channels);
  for (size_t i=0; i<stream_args.size(); i++) {
    stream_args[i] = std::string("fc32");

    stream_args[i].channels = {i};
    data->rx_stream[i] = data->usrp->get_rx_stream(stream_args[i]);
    uhd::stream_cmd_t stream_cmd(uhd::stream_cmd_t::STREAM_MODE_START_CONTINUOUS);
    stream_cmd.num_samps = 0;
    data->rx_stream[i]->issue_stream_cmd(stream_cmd);
  }

  // Launch the receive thread
  data->stop_signal = false;
  data->rx_thread = new std::thread(&X310Driver_ReceiveLoop, handle);
}

void X310Driver_StopRxStream(X310DriverHandle handle, uint32_t tunerIndex) {
  X310DriverData* data = (X310DriverData*)handle;
  data->stop_signal = true;
  std::this_thread::sleep_for(std::chrono::milliseconds(STOP_TIME)); // Give some time for the thread to stop
  if (data->rx_thread && data->rx_thread->joinable()) {
    data->rx_thread->join();
    delete data->rx_thread;
    data->rx_thread = nullptr;
    data->stop_signal = false;
  }

  for (size_t i=0; i<data->num_rx_channels; i++) {
    data->rx_stream[i]->issue_stream_cmd(uhd::stream_cmd_t::STREAM_MODE_STOP_CONTINUOUS);
    data->rx_stream[i] = nullptr;
  }
}

uint32_t X310Driver_GetTxFrequency(X310DriverHandle handle, uint32_t tunerIndex) {
  X310DriverData* data = (X310DriverData*)handle;
    return (uint32_t)data->usrp->get_tx_freq(tunerIndex);
}

void X310Driver_SetTxFrequency(X310DriverHandle handle, uint32_t tunerIndex, uint32_t freq) {
  X310DriverData* data = (X310DriverData*)handle;
   data->usrp->set_tx_freq(freq, tunerIndex);
}

void X310Driver_StartTxStream(X310DriverHandle handle, uint32_t tunerIndex) {
  X310DriverData* data = (X310DriverData*)handle;
  //Implement Start TX Stream
}

void X310Driver_StopTxStream(X310DriverHandle handle, uint32_t tunerIndex) {
  X310DriverData* data = (X310DriverData*)handle;
  //Implement Stop TX Stream
}

void X310Driver_Close(X310DriverHandle handle) {
    X310DriverData* data = (X310DriverData*)handle;
    delete data;
}

float X310Driver_GetLatitude(X310DriverHandle handle) {
  X310DriverData* data = (X310DriverData*)handle;
  // make sure the GPSDO really has lock, otherwise you'll only get zeros
  if (!data->usrp->get_mboard_sensor("gps_locked").to_bool())
    return 0.0f;

  const auto gga = data->usrp->get_mboard_sensor("gps_gpgga");
  // `value` holds the raw NMEA sentence without the pretty‑print prefix
  auto gps = parse_gpgga(gga.value);
  return gps.lat;
}

float X310Driver_GetLongitude(X310DriverHandle handle) {
  X310DriverData* data = (X310DriverData*)handle;
  // make sure the GPSDO really has lock, otherwise you'll only get zeros
  if (!data->usrp->get_mboard_sensor("gps_locked").to_bool())
    return 0.0f;

  const auto gga = data->usrp->get_mboard_sensor("gps_gpgga");
  // `value` holds the raw NMEA sentence without the pretty‑print prefix
  auto gps = parse_gpgga(gga.value);
  return gps.lon;
}

float X310Driver_GetElevation(X310DriverHandle handle) {
  X310DriverData* data = (X310DriverData*)handle;
  // make sure the GPSDO really has lock, otherwise you'll only get zeros
  if (!data->usrp->get_mboard_sensor("gps_locked").to_bool())
    return 0.0f;

  const auto gga = data->usrp->get_mboard_sensor("gps_gpgga");
  // `value` holds the raw NMEA sentence without the pretty‑print prefix
  auto gps = parse_gpgga(gga.value);
  return gps.alt;
}

// TODO: profile x310 sample rate and overflow error handle
void X310Driver_ReceiveLoop(X310DriverHandle handle) {
  X310DriverData* data = (X310DriverData*)handle;
  uhd::rx_metadata_t md;

  std::vector<size_t> stream_maxes(data->num_rx_channels);
  for (size_t i=0; i<stream_maxes.size(); i++) {
    stream_maxes[i] = data->rx_stream[i]->get_max_num_samps();
  }

  auto max_samps_stream= std::max_element(stream_maxes.begin(), stream_maxes.end());
  size_t samps_per_buff = *max_samps_stream;

  std::vector<std::vector<std::complex<float>>> buffs(data->num_rx_channels, std::vector<std::complex<float>>(samps_per_buff));
  data->power_dbs.resize(data->num_rx_channels, std::vector<float>(samps_per_buff));

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
    // TODO Magic number
    // std::vector<size_t> num_rx_samps(data->num_rx_channels);
    size_t num_rx_samps[data->num_rx_channels];

    for (size_t i=0; i<data->num_rx_channels; i++) {
      num_rx_samps[i] = data->rx_stream[i]->recv(buff_ptrs[i], samps_per_buff, md, 1.0);
      if (md.error_code != uhd::rx_metadata_t::ERROR_CODE_NONE) {
        std::cerr << "Tuner " << i << " Receive error: " << md.strerror() << std::endl;
        continue;
      }
    }

    for(size_t i=0; i<data->num_rx_channels; i++) {
      size_t fft_size = num_rx_samps[i];
      if (fft_size == 0) { continue; }

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
      data->fft_mutex[i].lock(); // Protect shared host buffer access
      // Ensure data->ffts[i] is sized correctly (fft_size)
      if (data->power_dbs[i].size() != fft_size) {
          data->power_dbs[i].resize(fft_size); // Resize if necessary
      }
      CUDA_CHECK(cudaMemcpy(data->power_dbs[i].data(), d_power_db_shifted, power_buffer_size_bytes, cudaMemcpyDeviceToHost));
      data->fft_mutex[i].unlock();
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

void X310Driver_LockFFTMutex(X310DriverHandle driver_handle, uint32_t index) {
  X310DriverData* driver_data = (X310DriverData*)driver_handle;
  driver_data->fft_mutex[index].lock();
}

void X310Driver_UnlockFFTMutex(X310DriverHandle driver_handle, uint32_t index) {
  X310DriverData* driver_data = (X310DriverData*)driver_handle;
  driver_data->fft_mutex[index].unlock();
}

uint32_t X310Driver_GetPowerDbsSize(X310DriverHandle handle, uint32_t tunerIndex) {
  X310DriverData* data = (X310DriverData*)handle;
  return data->power_dbs[tunerIndex].size();
}

void X310Driver_GetPowerDbsData(X310DriverHandle handle, uint32_t tunerIndex, float* data, uint32_t size) {
  X310DriverData* driver = (X310DriverData*)handle;
  memcpy(data, driver->power_dbs[tunerIndex].data(), size * sizeof(float));
}



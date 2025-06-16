#include "b20x_driver.h"
#include <iostream>
#include <uhd/utils/thread.hpp>
#include <uhd/utils/safe_main.hpp>
#include <uhd/usrp/multi_usrp.hpp>
#include <uhd/exception.hpp>
#include <boost/interprocess/shared_memory_object.hpp>
#include <boost/interprocess/mapped_region.hpp>
#include <boost/interprocess/sync/named_semaphore.hpp>
#include <stdint.h>
#include <thread>
#include <mutex>
#include <condition_variable>
#include <string>
#include <fstream>
#include <cmath>

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

struct b20xDriverData {
  uhd::usrp::multi_usrp::sptr usrp;
  uhd::rx_streamer::sptr rx_stream[1];
  std::thread *rx_thread = nullptr;
  bool stop_signal = false;
  std::mutex fft_mutex;
  std::vector<std::vector<float>> power_dbs;
  bool coherent_mode = false;
};

// struct that will hold data for the circ file buffer
struct circ_file_info_obj
{
  // file stream handle
  std::fstream* file_pointer;
  // stores the next index in the for writing
  uint64_t write_ptr;
  // stores the write chunk size bytes
  uint64_t write_size_bytes;
  // stores the size of the circ buffer in bytes
  uint64_t circ_buff_size_bytes;
};
// shared mem for circ file info
// made of circ file info objs
std::vector<circ_file_info_obj> circ_file_info;

// function to handle file write
// will be spun off as a thread so the file write can occure parallel to cudaFFT
// TODO: reverse this; FFT should be a seperate thread and file write should be the priority
void b20xDriver_CircFileBuffFunct(std::vector<circ_file_info_obj>& buff_info, std::vector<std::complex<float>*> buff_ptrs) {
  // loop and write each stream to their files
  for (int i = 0; i < buff_info.size(); i++)
  {
    // test for file overflow
    if ((buff_info[i].write_ptr + buff_info[i].write_size_bytes) > buff_info[i].circ_buff_size_bytes)
    {
      // fill and remainder values
      uint64_t fill_bytes = buff_info[i].circ_buff_size_bytes - buff_info[i].write_ptr;
      uint64_t remainder_bytes = buff_info[i].write_size_bytes - fill_bytes;
      // write fill
      buff_info[i].file_pointer->write(reinterpret_cast<const char*>(buff_ptrs[i]), fill_bytes);
      buff_info[i].file_pointer->seekp(0, std::ios::beg);
      // write remainder
      buff_info[i].file_pointer->write(reinterpret_cast<const char*>(buff_ptrs[i]+fill_bytes), remainder_bytes);
      buff_info[i].write_ptr = remainder_bytes;
      buff_info[i].file_pointer->seekp(buff_info[i].write_ptr, std::ios::beg);
      return;
    }
    // no file overflow
    buff_info[i].file_pointer->write(reinterpret_cast<const char*>(buff_ptrs[i]), buff_info[i].write_size_bytes);
    // adjust write index
    // add write size to current pointer
    // mod by buffer size
    buff_info[i].write_ptr = (buff_info[i].write_ptr + buff_info[i].write_size_bytes) % buff_info[i].circ_buff_size_bytes;
    buff_info[i].file_pointer->seekp(buff_info[i].write_ptr, std::ios::beg);
  }
  return;
}

b20xDriverHandle b20xDriver_Init(const char* args) {
    b20xDriverData* data = new b20xDriverData();
    try {
        data->usrp = uhd::usrp::multi_usrp::make(std::string(args));
    } catch (uhd::exception& e) {
        std::cerr << "b20x Error: " << e.what() << std::endl;
        delete data;
        return nullptr;
    }
    return (b20xDriverHandle)data;
}

uint32_t b20xDriver_GetNumTuners(b20xDriverHandle handle) {
  b20xDriverData* data = (b20xDriverData*)handle;
  return data->usrp->get_rx_num_channels();
}

uint32_t b20xDriver_GetRxFrequency(b20xDriverHandle handle, uint32_t tunerIndex) {
  b20xDriverData* data = (b20xDriverData*)handle;
  return (uint32_t)data->usrp->get_rx_freq(tunerIndex);
}

void b20xDriver_SetRxFrequency(b20xDriverHandle handle, uint32_t tunerIndex, uint32_t freq) {
  b20xDriverData* data = (b20xDriverData*)handle;
  data->usrp->set_rx_freq(freq, tunerIndex);
}

uint32_t b20xDriver_GetSampleRate(b20xDriverHandle handle, uint32_t tunerIndex) {
  b20xDriverData* data = (b20xDriverData*)handle;
  return (uint32_t)data->usrp->get_rx_rate(tunerIndex);
}

void b20xDriver_SetSampleRate(b20xDriverHandle handle, uint32_t tunerIndex, uint32_t rate) {
  b20xDriverData* data = (b20xDriverData*)handle;
  data->usrp->set_rx_rate(rate, tunerIndex);
}

uint32_t b20xDriver_GetRxBandwidth(b20xDriverHandle handle, uint32_t tunerIndex) {
  b20xDriverData* data = (b20xDriverData*)handle;
  return (uint32_t)data->usrp->get_rx_bandwidth(tunerIndex);
}

void b20xDriver_SetRxBandwidth(b20xDriverHandle handle, uint32_t tunerIndex, uint32_t bandwidth) {
  b20xDriverData* data = (b20xDriverData*)handle;
  data->usrp->set_rx_bandwidth(bandwidth, tunerIndex);
}

uint32_t b20xDriver_GetGain(b20xDriverHandle handle, uint32_t tunerIndex) {
  b20xDriverData* data = (b20xDriverData*)handle;
  return (uint32_t)data->usrp->get_rx_gain(tunerIndex);
}

void b20xDriver_SetGain(b20xDriverHandle handle, uint32_t tunerIndex, uint32_t gain) {
  b20xDriverData* data = (b20xDriverData*)handle;
  data->usrp->set_rx_gain(gain, tunerIndex);
}

bool b20xDriver_GetDcBias(b20xDriverHandle handle, uint32_t tunerIndex) {
  b20xDriverData* data = (b20xDriverData*)handle;
  //Implement DC bias getter
  return false;
}

void b20xDriver_SetDcBias(b20xDriverHandle handle, uint32_t tunerIndex, bool dcBias) {
   b20xDriverData* data = (b20xDriverData*)handle;
  //Implement DC bias setter
}

bool b20xDriver_GetAgc(b20xDriverHandle handle, uint32_t tunerIndex) {
  b20xDriverData* data = (b20xDriverData*)handle;
  //Implement AGC getter
  return false;
}

void b20xDriver_SetAgc(b20xDriverHandle handle, uint32_t tunerIndex, bool agc) {
  b20xDriverData* data = (b20xDriverData*)handle;
  //Implement AGC setter
}

void b20xDriver_StartRxStream(b20xDriverHandle handle, uint32_t tunerIndex) {
  b20xDriverData* data = (b20xDriverData*)handle;

  uhd::stream_args_t stream_args0("fc32");

  // Setup stream 0
  stream_args0.channels = {0};
  data->rx_stream[0] = data->usrp->get_rx_stream(stream_args0);
  uhd::stream_cmd_t stream_cmd0(uhd::stream_cmd_t::STREAM_MODE_START_CONTINUOUS);
  stream_cmd0.num_samps = 0;
  data->rx_stream[0]->issue_stream_cmd(stream_cmd0);

  // Launch the receive thread
  data->stop_signal = false;
  data->rx_thread = new std::thread(&b20xDriver_ReceiveLoop, handle);
}

void b20xDriver_StopRxStream(b20xDriverHandle handle, uint32_t tunerIndex) {
  b20xDriverData* data = (b20xDriverData*)handle;
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
}

uint32_t b20xDriver_GetTxFrequency(b20xDriverHandle handle, uint32_t tunerIndex) {
  b20xDriverData* data = (b20xDriverData*)handle;
    return (uint32_t)data->usrp->get_tx_freq(tunerIndex);
}

void b20xDriver_SetTxFrequency(b20xDriverHandle handle, uint32_t tunerIndex, uint32_t freq) {
  b20xDriverData* data = (b20xDriverData*)handle;
   data->usrp->set_tx_freq(freq, tunerIndex);
}

void b20xDriver_StartTxStream(b20xDriverHandle handle, uint32_t tunerIndex) {
  b20xDriverData* data = (b20xDriverData*)handle;
  //Implement Start TX Stream
}

void b20xDriver_StopTxStream(b20xDriverHandle handle, uint32_t tunerIndex) {
  b20xDriverData* data = (b20xDriverData*)handle;
  //Implement Stop TX Stream
}

void b20xDriver_Close(b20xDriverHandle handle) {
    b20xDriverData* data = (b20xDriverData*)handle;
    delete data;
}

float b20xDriver_GetLatitude(b20xDriverHandle handle) {
  b20xDriverData* data = (b20xDriverData*)handle;
  // make sure the GPSDO really has lock, otherwise you'll only get zeros
  // NOTE: gps_locked sensor not found on b200
  return 0.0f;
  const auto gga = data->usrp->get_mboard_sensor("gps_gpgga");
  return 0;
}

float b20xDriver_GetLongitude(b20xDriverHandle handle) {
  b20xDriverData* data = (b20xDriverData*)handle;
  // make sure the GPSDO really has lock, otherwise you'll only get zeros
  // NOTE: gps_locked sensor not found on b200
  return 0.0f;

  const auto gga = data->usrp->get_mboard_sensor("gps_gpgga");
  return 0;
}

float b20xDriver_GetElevation(b20xDriverHandle handle) {
  b20xDriverData* data = (b20xDriverData*)handle;
  // make sure the GPSDO really has lock, otherwise you'll only get zeros
  // NOTE: gps_locked sensor not found on b200
  return 0.0f;

  const auto gga = data->usrp->get_mboard_sensor("gps_gpgga");
  return 0;
}

std::fstream file;
std::string file_name = "/tmp/test_file";

void b20xDriver_ReceiveLoop(b20xDriverHandle handle) {
  b20xDriverData* data = (b20xDriverData*)handle;
  uhd::rx_metadata_t md;
  size_t samps_per_buff = 0;
  const size_t num_channels = 1;
  //size_t stream0_max = data->rx_stream[0]->get_max_num_samps();
  size_t stream0_max = 1024;
  samps_per_buff = stream0_max;

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

  // circular file buffer init code
  // for every RX stream, add a new circ_file_buff obj
  // init values and resize vector
  // TODO: make the total file buffer size not hard coded
  float file_record_seconds = 5.0;
  for (int i = 0; i < num_channels; i++)
  {
    uint64_t buff_count = std::round((file_record_seconds * data->usrp->get_rx_rate(i))/samps_per_buff);
    circ_file_info_obj tmp;
    tmp.write_ptr = 0;
    // make one file per stream
    std::string file_name = "/tmp/.LTU_B20x_circ_file_buff_"+std::to_string(i);
    tmp.file_pointer = new std::fstream;
    tmp.file_pointer->open(file_name, std::ios::out | std::ios::binary | std::ios::trunc);
    tmp.write_size_bytes = samps_per_buff * sizeof(std::complex<float>);
    tmp.circ_buff_size_bytes = buff_count * samps_per_buff * sizeof(std::complex<float>);
    circ_file_info.push_back(tmp);
  }

  std::cout << "file buffer size in bytes: " << std::to_string(circ_file_info[0].circ_buff_size_bytes) << std::endl;

  // start the file write thread
  while (!data->stop_signal) {
    // note: for B200 and B205, only have 1 channel/tuner
    size_t num_rx_samps[1];

    // note: UHD documentation days this is non-blocking unless timeout
    // however, it will also return early if there is an error code with a fragmented packet len
    num_rx_samps[0] = data->rx_stream[0]->recv(buff_ptrs[0], samps_per_buff, md, 1.0);

    if (md.error_code != uhd::rx_metadata_t::ERROR_CODE_NONE) {
      std::cerr << "Tuner 0 Receive error: " << md.strerror() << std::endl;
      continue;
    }

    // error check for incorrect packet length
    // can't do post-process on partial packets
    if (num_rx_samps[0] == samps_per_buff)
    {
      // pass on real RX samps received value
      circ_file_info[0].write_size_bytes = num_rx_samps[0] * sizeof(std::complex<float>);

      // buff_ptrs has raw sample data, start file write thread
      std::thread write_thread(b20xDriver_CircFileBuffFunct, std::ref(circ_file_info), buff_ptrs);

      // cuda fft
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
      write_thread.join();
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

uint32_t b20xDriver_GetPowerDbsSize(b20xDriverHandle handle, uint32_t tunerIndex) {
  b20xDriverData* data = (b20xDriverData*)handle;
  return data->power_dbs[tunerIndex].size();
}

void b20xDriver_GetPowerDbsData(b20xDriverHandle handle, uint32_t tunerIndex, float* data, uint32_t size) {
  b20xDriverData* driver = (b20xDriverData*)handle;
  driver->fft_mutex.lock();
  memcpy(data, driver->power_dbs[tunerIndex].data(), size * sizeof(float));
  driver->fft_mutex.unlock();
}




#ifndef   X40_CONSTANTS_HPP
#define X40_CONSTANTS_HPP

#include <cstddef>

constexpr size_t PKT_LEN = 1024;
constexpr size_t X40_PACKET_SAMPLE_COUNT = 1018;
constexpr size_t X40_PACKET_SAMPLE_COUNT_PADDED = 1024; // pad to power of 2 for FFT and other operations

constexpr int NUM_USEC_IN_MS = 1000;
constexpr float GAIN_DB_PER_INDEX = 0.5;
constexpr uint8_t DB_GAIN_MAX = 30;
constexpr uint8_t DB_GAIN_MIN = 0;
constexpr int USECONDS_IN_SECOND = 1000000;

constexpr const char* ZYNQ_IP = "192.168.55.100";
constexpr const char* ZYNQ_GPSDO_PORT = "2947";

#endif // X40_CONSTANTS_HPP
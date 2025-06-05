
#ifndef GPS_UTILS_H
#define GPS_UTILS_H

#include <cstdint>
#include <ctime>

#ifdef __cplusplus
extern "C" {
#endif



typedef struct GPSInfo {
    float latitude;
    float longitude;
    float altitude;
    timespec gpsTime;

} GPSInfo;

uint32_t getGPSInfo(const char* gpsdHost, const char* gpsdPort, GPSInfo *gpsInfo);
void gpsPollLoop(const char *gpsdHost, const char *gpsdPort, const volatile bool &keepRunning, GPSInfo *gpsInfo, uint32_t pollPeriodUS);

uint64_t timespecToNanos(timespec timespecIn);

#ifdef __cplusplus
}
#endif

#endif // GPS_UTILS_H
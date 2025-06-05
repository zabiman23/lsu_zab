#include <cstdint>
#ifndef GPS_UTILS_H
#define GPS_UTILS_H

#ifdef __cplusplus
extern "C" {
#endif


typedef struct GPSInfo {
    float latitude;
    float longitude;
    float altitude;

} GPSInfo;

uint32_t getGPSInfo(const char* gpsdHost, const char* gpsdPort, GPSInfo *gpsInfo);
void gpsPollLoop(const char *gpsdHost, const char *gpsdPort, const bool &keepRunning, GPSInfo *gpsInfo);

#ifdef __cplusplus
}
#endif

#endif // GPS_UTILS_H
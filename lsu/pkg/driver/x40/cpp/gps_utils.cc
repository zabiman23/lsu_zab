#include <cerrno>
#include <math.h>
#include <unistd.h>
#include <gps.h>
#include "gps_utils.h"
#include <string>
#include <stdexcept>

#define MODE_STR_NUM 4
static std::string mode_str[MODE_STR_NUM] = {
    "n/a",
    "None",
    "2D",
    "3D"
};



void debugDump(gps_data_t *gpsdata){
    fprintf(stderr,"Longitude: %lf\nLatitude: %lf\nAltitude: %lf\nAccuracy: %lf\n\n",
                gpsdata->fix.latitude, gpsdata->fix.longitude, gpsdata->fix.altitude,
                (gpsdata->fix.epx>gpsdata->fix.epy)?gpsdata->fix.epx:gpsdata->fix.epy);
}

uint64_t timespecToNanos(timespec timeSpecIn){
    uint64_t seconds = timeSpecIn.tv_sec;
    uint32_t nanoseconds = timeSpecIn.tv_nsec;
    // Ensure nanoseconds are within the valid range [0, 999,999,999]
    // if (nanoseconds < 0 || nanoseconds >= 1000000000) {
    //     throw std::out_of_range("Nanoseconds value is out of range");
    // }

    // Convert nanoseconds to the fractional part of a second (as a double)
    double fractionalSeconds = static_cast<double>(nanoseconds) / 1000000000.0;

    // Combine seconds and the fractional part
    double combinedDouble = static_cast<double>(seconds) + fractionalSeconds;

    // Scale to a suitable integer representation (e.g., nanoseconds since epoch)
    // Choose a scaling factor that fits within a 64-bit integer and provides
    // sufficient precision. Nanoseconds is a common choice.
    uint64_t integerTimestamp = static_cast<uint64_t>(combinedDouble * 1000000000.0);
    return integerTimestamp;
}

uint32_t getGPSInfo(const char* gpsdHost, const char* gpsdPort, GPSInfo *gpsInfo){
    gps_data_t gpsdata;

    //connect to GPSd
    if(gps_open(gpsdHost, gpsdPort, &gpsdata) < 0){
        fprintf(stderr,"Could not connect to GPSd\n");
        return(-1);
    }

    //register for updates
    gps_stream(&gpsdata, WATCH_ENABLE | WATCH_JSON, NULL);
    
    fprintf(stderr,"Waiting for gps lock.");
    //when status is >0, you have data.
    while(gpsdata.status==0){
        //block for up to .5 seconds
        if (gps_waiting(&gpsdata, 500)){
            //dunno when this would happen but its an error
            if(gps_read(&gpsdata, NULL, 0)==-1){
                fprintf(stderr,"GPSd Error\n");
                gps_stream(&gpsdata, WATCH_DISABLE, NULL);
                gps_close(&gpsdata);
                return(ENODATA);
                break;
            }
            else{
                //status>0 means you have data
                if(gpsdata.status>0){
                    //sometimes if your GPS doesnt have a fix, it sends you data anyways
                    //the values for the fix are NaN. this is a clever way to check for NaN.
                    if(gpsdata.fix.longitude != gpsdata.fix.longitude || gpsdata.fix.altitude != gpsdata.fix.altitude){
                        fprintf(stderr,"Could not get a GPS fix.\n");
                        gps_stream(&gpsdata, WATCH_DISABLE, NULL);
                        gps_close(&gpsdata);
                        return(ENODATA);
                    }
                    //otherwise you have a legitimate fix!
                    else
                        fprintf(stderr,"\n");
                }
                //if you don't have any data yet, keep waiting for it.
                else
                    fprintf(stderr,".");
            }
        }
        //apparently gps_stream disables itself after a few seconds.. in this case, gps_waiting returns false.
        //we want to re-register for updates and keep looping! we dont have a fix yet.
        else
            gps_stream(&gpsdata, WATCH_ENABLE | WATCH_JSON, NULL);

        //just a sleep for good measure.
        sleep(1);
    }
    debugDump(&gpsdata);
    
    gpsInfo->latitude = gpsdata.fix.latitude;
    gpsInfo->longitude = gpsdata.fix.longitude;
    gpsInfo->altitude = gpsdata.fix.altitude;
    gpsInfo->gpsTime = gpsdata.fix.time;

    //cleanup
    gps_stream(&gpsdata, WATCH_DISABLE, NULL);
    gps_close(&gpsdata);

    return 0;
}



void gpsPollLoop(const char *gpsdHost, const char *gpsdPort, const volatile bool &keepRunning, GPSInfo *gpsInfo, uint32_t pollPeriodUS)
{ 
    gps_data_t gpsData;

    if (0 != gps_open(gpsdHost, gpsdPort, &gpsData)) {
        printf("Open error.  Bye, bye\n");
        return;
    }    

    gps_stream(&gpsData, WATCH_ENABLE | WATCH_JSON, NULL);

    while(keepRunning){
        while (gps_waiting(&gpsData, 5000000)) {
            if (-1 == gps_read(&gpsData, NULL, 0)) {
                printf("Read error.  Bye, bye\n");
                break;
            }
            if (MODE_SET != (MODE_SET & gpsData.set)) {
                // did not even get mode, nothing to see here
                continue;
            }
            if (0 > gpsData.fix.mode ||
                MODE_STR_NUM <= gpsData.fix.mode) {
                gpsData.fix.mode = 0;
            }
            // printf("Fix mode: %s (%d) Time: ",
            //     mode_str[gpsData.fix.mode],
            //     gpsData.fix.mode);
            if (TIME_SET == (TIME_SET & gpsData.set)) {
                // not 32 bit safe
                // printf("%ld.%09ld ", gpsData.fix.time.tv_sec,
                //     gpsData.fix.time.tv_nsec);
            } else {
                puts("n/a ");
            }
            if (isfinite(gpsData.fix.latitude) &&
                isfinite(gpsData.fix.longitude)) {
                
                // printf("Lat %.6f Lon %.6f\n",
                //     gpsData.fix.latitude, gpsData.fix.longitude);
                gpsInfo->altitude = gpsData.fix.altitude;
                gpsInfo->latitude = gpsData.fix.latitude;
                gpsInfo->longitude = gpsData.fix.longitude;
                gpsInfo->gpsTime = gpsData.fix.time;

            } else {
                // printf("Lat n/a Lon n/a\n");
            }
        }
        // sleep for pollPeriodUS microseconds
        usleep(pollPeriodUS);
    }

    printf("Exiting GPS poll loop\n");
    gps_stream(&gpsData, WATCH_DISABLE, NULL);
    gps_close(&gpsData);
    
}

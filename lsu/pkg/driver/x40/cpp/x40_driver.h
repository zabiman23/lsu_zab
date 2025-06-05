#ifndef X40_DRIVER_H
#define X40_DRIVER_H

#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef void* X40DriverHandle;

X40DriverHandle X40Driver_Init(const char* args);
void X40Driver_Close(X40DriverHandle handle);

uint32_t X40Driver_GetNumTuners(X40DriverHandle handle);
uint32_t X40Driver_GetNumCoherentTuners(X40DriverHandle handle);

uint32_t X40Driver_GetRxFrequency(X40DriverHandle handle, uint32_t tunerIndex);
void X40Driver_SetRxFrequency(X40DriverHandle handle, uint32_t tunerIndex, uint32_t freq);
uint32_t X40Driver_GetSampleRate(X40DriverHandle handle, uint32_t tunerIndex);
void X40Driver_SetSampleRate(X40DriverHandle handle, uint32_t tunerIndex, uint32_t rate);
uint32_t X40Driver_GetRxBandwidth(X40DriverHandle handle, uint32_t tunerIndex);
void X40Driver_SetRxBandwidth(X40DriverHandle handle, uint32_t tunerIndex, uint32_t bandwidth);
uint32_t X40Driver_GetGain(X40DriverHandle handle, uint32_t tunerIndex);
void X40Driver_SetGain(X40DriverHandle handle, uint32_t tunerIndex, uint32_t gain);
bool X40Driver_GetDcBias(X40DriverHandle handle, uint32_t tunerIndex);
void X40Driver_SetDcBias(X40DriverHandle handle, uint32_t tunerIndex, bool dcBias);
bool X40Driver_GetAgc(X40DriverHandle handle, uint32_t tunerIndex);
void X40Driver_SetAgc(X40DriverHandle handle, uint32_t tunerIndex, bool agc);
void X40Driver_StartRxStream(X40DriverHandle handle, uint32_t tunerIndex);
void X40Driver_StopRxStream(X40DriverHandle handle, uint32_t tunerIndex);

float X40Driver_GetLatitude(X40DriverHandle handle);
float X40Driver_GetLongitude(X40DriverHandle handle);
float X40Driver_GetElevation(X40DriverHandle handle);

uint32_t X40Driver_GetTxFrequency(X40DriverHandle handle, uint32_t tunerIndex);
void X40Driver_SetTxFrequency(X40DriverHandle handle, uint32_t tunerIndex, uint32_t freq);
void X40Driver_StartTxStream(X40DriverHandle handle, uint32_t tunerIndex);
void X40Driver_StopTxStream(X40DriverHandle handle, uint32_t tunerIndex);

uint32_t X40Driver_GetPowerDbsSize(X40DriverHandle handle, uint32_t tunerIndex);
void X40Driver_GetPowerDbsData(X40DriverHandle handle, uint32_t tunerIndex, float* data, uint32_t size);

#ifdef __cplusplus
}
#endif

#endif // X40_DRIVER_H
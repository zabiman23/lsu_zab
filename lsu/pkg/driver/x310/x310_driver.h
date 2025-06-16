#ifndef B210_DRIVER_H
#define B210_DRIVER_H

#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef void* X310DriverHandle;

X310DriverHandle X310Driver_Init(const char* args);
void X310Driver_Close(X310DriverHandle handle);

uint32_t X310Driver_GetNumTuners(X310DriverHandle handle);
uint32_t X310Driver_GetNumCoherentTuners(X310DriverHandle handle);

uint32_t X310Driver_GetRxFrequency(X310DriverHandle handle, uint32_t tunerIndex);
void X310Driver_SetRxFrequency(X310DriverHandle handle, uint32_t tunerIndex, uint32_t freq);
uint32_t X310Driver_GetSampleRate(X310DriverHandle handle, uint32_t tunerIndex);
void X310Driver_SetSampleRate(X310DriverHandle handle, uint32_t tunerIndex, uint32_t rate);
uint32_t X310Driver_GetRxBandwidth(X310DriverHandle handle, uint32_t tunerIndex);
void X310Driver_SetRxBandwidth(X310DriverHandle handle, uint32_t tunerIndex, uint32_t bandwidth);
uint32_t X310Driver_GetGain(X310DriverHandle handle, uint32_t tunerIndex);
void X310Driver_SetGain(X310DriverHandle handle, uint32_t tunerIndex, uint32_t gain);
bool X310Driver_GetDcBias(X310DriverHandle handle, uint32_t tunerIndex);
void X310Driver_SetDcBias(X310DriverHandle handle, uint32_t tunerIndex, bool dcBias);
bool X310Driver_GetAgc(X310DriverHandle handle, uint32_t tunerIndex);
void X310Driver_SetAgc(X310DriverHandle handle, uint32_t tunerIndex, bool agc);
void X310Driver_StartRxStream(X310DriverHandle handle, uint32_t tunerIndex);
void X310Driver_StopRxStream(X310DriverHandle handle, uint32_t tunerIndex);

float X310Driver_GetLatitude(X310DriverHandle handle);
float X310Driver_GetLongitude(X310DriverHandle handle);
float X310Driver_GetElevation(X310DriverHandle handle);

uint32_t X310Driver_GetTxFrequency(X310DriverHandle handle, uint32_t tunerIndex);
void X310Driver_SetTxFrequency(X310DriverHandle handle, uint32_t tunerIndex, uint32_t freq);
void X310Driver_StartTxStream(X310DriverHandle handle, uint32_t tunerIndex);
void X310Driver_StopTxStream(X310DriverHandle handle, uint32_t tunerIndex);

void X310Driver_ReceiveLoop(X310DriverHandle handle);
uint32_t X310Driver_GetPowerDbsSize(X310DriverHandle handle, uint32_t tunerIndex);
void X310Driver_GetPowerDbsData(X310DriverHandle handle, uint32_t tunerIndex, float* data, uint32_t size);

void X310Driver_LockFFTMutex(X310DriverHandle handle, uint32_t index);
void X310Driver_UnlockFFTMutex(X310DriverHandle handle, uint32_t index);

#ifdef __cplusplus
}
#endif

#endif // X310_DRIVER_H

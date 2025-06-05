#ifndef B210_DRIVER_H
#define B210_DRIVER_H

#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef void* B210DriverHandle;

B210DriverHandle B210Driver_Init(const char* args);
void B210Driver_Close(B210DriverHandle handle);

uint32_t B210Driver_GetNumTuners(B210DriverHandle handle);
uint32_t B210Driver_GetNumCoherentTuners(B210DriverHandle handle);

uint32_t B210Driver_GetRxFrequency(B210DriverHandle handle, uint32_t tunerIndex);
void B210Driver_SetRxFrequency(B210DriverHandle handle, uint32_t tunerIndex, uint32_t freq);
uint32_t B210Driver_GetSampleRate(B210DriverHandle handle, uint32_t tunerIndex);
void B210Driver_SetSampleRate(B210DriverHandle handle, uint32_t tunerIndex, uint32_t rate);
uint32_t B210Driver_GetRxBandwidth(B210DriverHandle handle, uint32_t tunerIndex);
void B210Driver_SetRxBandwidth(B210DriverHandle handle, uint32_t tunerIndex, uint32_t bandwidth);
uint32_t B210Driver_GetGain(B210DriverHandle handle, uint32_t tunerIndex);
void B210Driver_SetGain(B210DriverHandle handle, uint32_t tunerIndex, uint32_t gain);
bool B210Driver_GetDcBias(B210DriverHandle handle, uint32_t tunerIndex);
void B210Driver_SetDcBias(B210DriverHandle handle, uint32_t tunerIndex, bool dcBias);
bool B210Driver_GetAgc(B210DriverHandle handle, uint32_t tunerIndex);
void B210Driver_SetAgc(B210DriverHandle handle, uint32_t tunerIndex, bool agc);
void B210Driver_StartRxStream(B210DriverHandle handle, uint32_t tunerIndex);
void B210Driver_StopRxStream(B210DriverHandle handle, uint32_t tunerIndex);

float B210Driver_GetLatitude(B210DriverHandle handle);
float B210Driver_GetLongitude(B210DriverHandle handle);
float B210Driver_GetElevation(B210DriverHandle handle);

uint32_t B210Driver_GetTxFrequency(B210DriverHandle handle, uint32_t tunerIndex);
void B210Driver_SetTxFrequency(B210DriverHandle handle, uint32_t tunerIndex, uint32_t freq);
void B210Driver_StartTxStream(B210DriverHandle handle, uint32_t tunerIndex);
void B210Driver_StopTxStream(B210DriverHandle handle, uint32_t tunerIndex);

void B210Driver_ReceiveLoop(B210DriverHandle handle);
uint32_t B210Driver_GetPowerDbsSize(B210DriverHandle handle, uint32_t tunerIndex);
void B210Driver_GetPowerDbsData(B210DriverHandle handle, uint32_t tunerIndex, float* data, uint32_t size);

#ifdef __cplusplus
}
#endif

#endif // B210_DRIVER_H
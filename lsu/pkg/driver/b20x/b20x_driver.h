#ifndef b20x_DRIVER_H
#define b20x_DRIVER_H

#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef void* b20xDriverHandle;

b20xDriverHandle b20xDriver_Init(const char* args);
void b20xDriver_Close(b20xDriverHandle handle);

uint32_t b20xDriver_GetNumTuners(b20xDriverHandle handle);
uint32_t b20xDriver_GetNumCoherentTuners(b20xDriverHandle handle);

uint32_t b20xDriver_GetRxFrequency(b20xDriverHandle handle, uint32_t tunerIndex);
void b20xDriver_SetRxFrequency(b20xDriverHandle handle, uint32_t tunerIndex, uint32_t freq);
uint32_t b20xDriver_GetSampleRate(b20xDriverHandle handle, uint32_t tunerIndex);
void b20xDriver_SetSampleRate(b20xDriverHandle handle, uint32_t tunerIndex, uint32_t rate);
uint32_t b20xDriver_GetRxBandwidth(b20xDriverHandle handle, uint32_t tunerIndex);
void b20xDriver_SetRxBandwidth(b20xDriverHandle handle, uint32_t tunerIndex, uint32_t bandwidth);
uint32_t b20xDriver_GetGain(b20xDriverHandle handle, uint32_t tunerIndex);
void b20xDriver_SetGain(b20xDriverHandle handle, uint32_t tunerIndex, uint32_t gain);
bool b20xDriver_GetDcBias(b20xDriverHandle handle, uint32_t tunerIndex);
void b20xDriver_SetDcBias(b20xDriverHandle handle, uint32_t tunerIndex, bool dcBias);
bool b20xDriver_GetAgc(b20xDriverHandle handle, uint32_t tunerIndex);
void b20xDriver_SetAgc(b20xDriverHandle handle, uint32_t tunerIndex, bool agc);
void b20xDriver_StartRxStream(b20xDriverHandle handle, uint32_t tunerIndex);
void b20xDriver_StopRxStream(b20xDriverHandle handle, uint32_t tunerIndex);

float b20xDriver_GetLatitude(b20xDriverHandle handle);
float b20xDriver_GetLongitude(b20xDriverHandle handle);
float b20xDriver_GetElevation(b20xDriverHandle handle);

uint32_t b20xDriver_GetTxFrequency(b20xDriverHandle handle, uint32_t tunerIndex);
void b20xDriver_SetTxFrequency(b20xDriverHandle handle, uint32_t tunerIndex, uint32_t freq);
void b20xDriver_StartTxStream(b20xDriverHandle handle, uint32_t tunerIndex);
void b20xDriver_StopTxStream(b20xDriverHandle handle, uint32_t tunerIndex);

void b20xDriver_ReceiveLoop(b20xDriverHandle handle);
uint32_t b20xDriver_GetPowerDbsSize(b20xDriverHandle handle, uint32_t tunerIndex);
void b20xDriver_GetPowerDbsData(b20xDriverHandle handle, uint32_t tunerIndex, float* data, uint32_t size);

#ifdef __cplusplus
}
#endif

#endif // b20x_DRIVER_H
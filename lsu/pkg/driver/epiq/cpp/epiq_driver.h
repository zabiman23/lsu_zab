#ifndef EPIQ_DRIVER_H
#define EPIQ_DRIVER_H

#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef void* EpiqDriverHandle;

EpiqDriverHandle EpiqDriver_Init(const char* args);
void EpiqDriver_Close(EpiqDriverHandle handle);

uint32_t EpiqDriver_GetNumTuners(EpiqDriverHandle handle);
uint32_t EpiqDriver_GetNumCoherentTuners(EpiqDriverHandle handle);

uint32_t EpiqDriver_GetRxFrequency(EpiqDriverHandle handle, uint32_t tunerIndex);
void EpiqDriver_SetRxFrequency(EpiqDriverHandle handle, uint32_t tunerIndex, uint32_t freq);
uint32_t EpiqDriver_GetSampleRate(EpiqDriverHandle handle, uint32_t tunerIndex);
void EpiqDriver_SetSampleRate(EpiqDriverHandle handle, uint32_t tunerIndex, uint32_t rate);
uint32_t EpiqDriver_GetRxBandwidth(EpiqDriverHandle handle, uint32_t tunerIndex);
void EpiqDriver_SetRxBandwidth(EpiqDriverHandle handle, uint32_t tunerIndex, uint32_t bandwidth);
uint32_t EpiqDriver_GetGain(EpiqDriverHandle handle, uint32_t tunerIndex);
void EpiqDriver_SetGain(EpiqDriverHandle handle, uint32_t tunerIndex, uint32_t gain);
bool EpiqDriver_GetDcBias(EpiqDriverHandle handle, uint32_t tunerIndex);
void EpiqDriver_SetDcBias(EpiqDriverHandle handle, uint32_t tunerIndex, bool dcBias);
bool EpiqDriver_GetAgc(EpiqDriverHandle handle, uint32_t tunerIndex);
void EpiqDriver_SetAgc(EpiqDriverHandle handle, uint32_t tunerIndex, bool agc);
void EpiqDriver_StartRxStream(EpiqDriverHandle handle, uint32_t tunerIndex);
void EpiqDriver_StopRxStream(EpiqDriverHandle handle, uint32_t tunerIndex);

float EpiqDriver_GetLatitude(EpiqDriverHandle handle);
float EpiqDriver_GetLongitude(EpiqDriverHandle handle);
float EpiqDriver_GetElevation(EpiqDriverHandle handle);

uint32_t EpiqDriver_GetTxFrequency(EpiqDriverHandle handle, uint32_t tunerIndex);
void EpiqDriver_SetTxFrequency(EpiqDriverHandle handle, uint32_t tunerIndex, uint32_t freq);
void EpiqDriver_StartTxStream(EpiqDriverHandle handle, uint32_t tunerIndex);
void EpiqDriver_StopTxStream(EpiqDriverHandle handle, uint32_t tunerIndex);

#ifdef __cplusplus
}
#endif

#endif // EPIQ_DRIVER_H
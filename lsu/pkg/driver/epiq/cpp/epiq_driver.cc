#include "epiq_driver.h"
#include <iostream>

#include <ctype.h>
#include <stdio.h>
#include <stdint.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>
#include <signal.h>
#include <errno.h>
#include <unistd.h>
#include <inttypes.h>
#include <libgen.h>
#include <fcntl.h>
#include <pthread.h>
#include <array>
#include <algorithm>

#include "sidekiq_api.h"
#include "gps_utils.h"
#include <thread>
// #include "arg_parser.h"


#define OUTPUT_PATH_MAX 100

#define NUM_USEC_IN_MS                      (1000)

#define MAX_COMMAND_SIZE                    (4000)

#ifndef SKIQ_MAX_NUM_CARDS
#    define SKIQ_MAX_NUM_CARDS              (32)
#endif

#define GAIN_DB_PER_INDEX                   (0.5)
#define DB_GAIN_MAX                         (30)
#define DB_GAIN_MIN                         (0)

/* Parameters passed to threads
   Instantiated by main() and passed to the threads
*/
typedef struct thread_params
{
    pthread_t                       receive_thread;  // thread responsible for receiving data
    struct radio_config*            p_rconfig;
    struct rx_radio_config*         p_rx_rconfig;
    uint32_t                        num_payload_words_to_acquire;
    char*                           p_file_path;
    uint8_t                         card_no;
    volatile bool                   init_complete;   // thread has completed init
    bool                            include_meta;
    bool                            perform_verify;
} thread_params;

#define THREAD_PARAMS_INITIALIZER                             \
{                                                             \
    .receive_thread                 = 0,                      \
    .p_rconfig                      = NULL,                   \
    .p_rx_rconfig                   = NULL,                   \
    .num_payload_words_to_acquire   = 0,                      \
    .p_file_path                    = NULL,                   \
    .card_no                        = UINT8_MAX,              \
    .init_complete                  = false,                  \
    .include_meta                   = DEFAULT_INCLUDE_META,   \
    .perform_verify                 = DEFAULT_PERFORM_VERIFY, \
}

/* Local variables for each thread
*/
typedef struct thread_variables
{
    FILE*               output_fp;
    uint32_t            total_num_payload_words_acquired;
    uint32_t            rx_block_cnt;
    uint32_t*           p_next_write;
    uint32_t*           p_rx_data;
    uint32_t*           p_rx_data_start;
    uint32_t            words_received;
    char *              p_file_path;
    bool                last_block;
} thread_variables;

#define THREAD_VARIABLES_INITIALIZER                            \
{                                                               \
    .output_fp                          = NULL,                 \
    .total_num_payload_words_acquired   = 0,                    \
    .rx_block_cnt                       = 0,                    \
    .p_next_write                       = NULL,                 \
    .p_rx_data_start                    = NULL,                 \
    .last_block                         = false,                \
    .p_file_path                        = NULL,                 \
    .words_received                     = 0,                    \
}

typedef struct rx_stats
{
    uint64_t            curr_rf_ts;         // current RF timestamp
    uint64_t            next_rf_ts;         // next RF timestamp
    uint64_t            first_rf_ts;        // first RF timestamp
    uint64_t            last_rf_ts;         // last RF timestamp
    uint64_t            first_sys_ts;       // first system timestamp
    uint64_t            last_sys_ts;        // last system timestamp
    uint32_t            nr_blocks_to_write;  // the requested number of blocks to be received
    bool                first_block;        // indicates if the first block has been received
} rx_stats;

#define RX_STATS_INITIALIZER    \
{                               \
    .curr_rf_ts         = 0,    \
    .next_rf_ts         = 0,    \
    .first_rf_ts        = 0,    \
    .last_rf_ts         = 0,    \
    .first_sys_ts       = 0,    \
    .last_sys_ts        = 0,    \
    .nr_blocks_to_write = 0,    \
    .first_block        = true, \
} 




/**
 * HARDCODED for X40 right now
 */
typedef struct EpiqDriverData {   
    uint8_t cardNum;
    std::string cardSerial;     
    const char *zynqIP;
    const char *zynqGPSDPort;
    std::thread *gpsThread;
    GPSInfo gpsInfo;
    volatile bool keepRunning;
    EpiqDriverData(uint8_t cNum, char *cSerial, const char *zynqIP, const char *zynqGPSDPort): 
             cardNum(cNum), cardSerial(cSerial), zynqIP(zynqIP), zynqGPSDPort(zynqGPSDPort),
             keepRunning(true)
    {
        gpsThread = new std::thread(gpsPollLoop, zynqIP, zynqGPSDPort, 
            keepRunning, &gpsInfo);
    }
    ~EpiqDriverData(){
        keepRunning = false;
        if(gpsThread != nullptr){
            gpsThread->join();
            delete gpsThread;
        }
    }
} EpiqDriverData;

//--------------------------------------------------------------------
// HELPER FUNCTIONS
//--------------------------------------------------------------------

/**
 * Gets the PROGRAMMED sample rate and bandwidth for the given tuner.
 */
uint32_t getProgrammedSRandBW(uint8_t cardNum, skiq_rx_hdl_t tunerHdl, uint32_t *sr, uint32_t *bw){    
    double actualSR;    
    uint32_t actualBW;
    int32_t retVal = skiq_read_rx_sample_rate_and_bandwidth(cardNum, tunerHdl, sr, &actualSR, bw, &actualBW);
    return retVal;
}

/**
 * Gets the ACTUAL sample rate and bandwidth for the given tuner.
 */
uint32_t getActualSRandBW(uint8_t cardNum, skiq_rx_hdl_t tunerHdl, double *sr, uint32_t *bw){    
    uint32_t progSR;    
    uint32_t progBW;
    int32_t retVal = skiq_read_rx_sample_rate_and_bandwidth(cardNum, tunerHdl, &progSR, sr, &progBW, bw);
    return retVal;
}

void getGainRangeSettings(uint8_t cardNum, skiq_rx_hdl_t tunerHdl){
    uint8_t gainIdxMin, gainIdxMax; 
    int32_t retVal = skiq_read_rx_gain_index_range (cardNum, tunerHdl, &gainIdxMin, &gainIdxMax);
    std::cout << "Retval: " << retVal << std::endl;
    std::cout << "Min index: " << static_cast<uint32_t>(gainIdxMin) << "; Max index: " << static_cast<uint32_t>(gainIdxMax) << "; Step: 1 idx = 0.5dB" << std::endl;
}

uint8_t getGainIdxFromGainDB(uint32_t gainValDB, uint8_t idxMin, uint8_t idxMax){
    /**
     * y = m*x + b; m = (Change in index) / (Change in gainValDB)
     *   m = (idx_max - idxMin) / (DB_GAIN_MAX - DB_GAIN_MIN)
     * gainValDB = 0 -> idx = idxMin so b = idxMin 
    */
   uint8_t gainValClipped = gainValDB > DB_GAIN_MAX ? DB_GAIN_MAX : static_cast<uint8_t>(gainValDB);
   float scaleFac = (idxMax - idxMin) / DB_GAIN_MAX; //DB_GAIN_MIN is 0
   uint8_t scaleFacInt = static_cast<uint8_t>(scaleFac);
   uint8_t idx = (scaleFacInt * gainValClipped) + idxMin;
   return idx;
}

uint32_t getGainDBFromGainIdx(uint8_t idx, uint8_t idxMin, uint8_t idxMax){
    /**
     * Using the math from the inverse function of this....
     * x = (y - b) / m 
    */
   uint8_t idxClipped = idx > idxMax ? idxMax : idx;
   float gainDB = (idxClipped - idxMin) * (DB_GAIN_MAX/(idxMax - idxMin));
   return static_cast<uint32_t>(gainDB);
}


//--------------------------------------------------------------------
// END HELPER FUNCTIONS
//--------------------------------------------------------------------

EpiqDriverHandle EpiqDriver_Init(const char* args) {
    std::cout << "EpiqDriver_Init called with args: " << args << std::endl;
    
    // Taken from an example...on the  X40 there's only 1 card
    uint8_t numCards = 0;
    uint8_t cardList[SKIQ_MAX_NUM_CARDS];
    uint8_t cardNum = 0;
    uint8_t i = 0;
    char *serialStr;
    
    skiq_get_cards( skiq_xport_type_auto, &numCards, cardList );
    for (i = 0; i < numCards; i++)
    {
        /* determine the serial number based on the card number */
        skiq_read_serial_string(cardList[i], &serialStr );
        printf("Sidekiq card number %u has serial number %s\n", cardList[i], serialStr);
    }
    /* determine the card number based on the serial number */
    skiq_get_card_from_serial_string( serialStr, &cardNum );
    printf("Sidekiq serial number %s is located at card number %u\n",
    serialStr, cardNum);

    skiq_xport_type_t xpType = skiq_xport_type_auto;
    skiq_xport_init_level_t xpInitLevel = skiq_xport_init_level_full;
    int32_t retVal = skiq_init(xpType, xpInitLevel, cardList, numCards);
    if(retVal != 0){
        return nullptr;
    }

    // Defaults to Q/I mode, we want I/Q mode
    skiq_write_iq_order_mode(cardNum, skiq_iq_order_iq);

    // TODO maybe pass tehs as args? They really don't change tho...
    const char *zynqIP = "192.168.55.100";
    const char *zynqGPSDPort = "2947";
    EpiqDriverData *driverHandle = new EpiqDriverData(cardNum, serialStr, zynqIP, zynqGPSDPort);  
    return (EpiqDriverHandle) driverHandle;
}



void EpiqDriver_Close(EpiqDriverHandle handle) {
    std::cout << "EpiqDriver_Close called" << std::endl;
    skiq_exit();
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    delete driverdata; 
}

uint32_t EpiqDriver_GetNumTuners(EpiqDriverHandle handle) {
    /*
    skiq_rx_hdl_A1=0,
    skiq_rx_hdl_A2=1,
    skiq_rx_hdl_B1=2,
    skiq_rx_hdl_B2=3,
    skiq_rx_hdl_C1=4,
    skiq_rx_hdl_D1=5,
    skiq_rx_hdl_end
     */
    std::cout << "EpiqDriver_GetNumTuners called" << std::endl;
    // Implement logic to get the number of tuners
    return skiq_rx_hdl_end; // Dummy value
}

uint32_t EpiqDriver_GetNumCoherentTuners(EpiqDriverHandle handle) {
    std::cout << "EpiqDriver_GetNumCoherentTuners called" << std::endl;
    // Coherent are A1+A2, and B1+B2 according to the docs
    return 4; 
}

uint32_t EpiqDriver_GetRxFrequency(EpiqDriverHandle handle, uint32_t tunerIndex) {
    std::cout << "EpiqDriver_GetRxFrequency called with tunerIndex: " << tunerIndex << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    uint64_t programmedFreq;
    double actualFreq;
    int32_t retVal = skiq_read_rx_LO_freq(driverdata->cardNum, tunerIndexHdl, &programmedFreq, &actualFreq);
    return (uint32_t)actualFreq;
}

void EpiqDriver_SetRxFrequency(EpiqDriverHandle handle, uint32_t tunerIndex, uint32_t freq) {
    std::cout << "EpiqDriver_SetRxFrequency called with tunerIndex: " << tunerIndex << " and freq: " << freq << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    uint32_t retVal = skiq_write_rx_LO_freq(driverdata->cardNum, tunerIndexHdl, (uint64_t) freq);
}

uint32_t EpiqDriver_GetSampleRate(EpiqDriverHandle handle, uint32_t tunerIndex) {
    std::cout << "EpiqDriver_GetSampleRate called with tunerIndex: " << tunerIndex << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    uint32_t programmedRate;
    double actualRate;
    int32_t retVal = skiq_read_rx_sample_rate(driverdata->cardNum, tunerIndexHdl, &programmedRate, &actualRate);
    std::cout << "Programmed Sample Rate for Tuner " << tunerIndex << ": " << programmedRate << std::endl;
    std::cout << "Actual Sample Rate for Tuner " << tunerIndex << ": " << actualRate << std::endl;
    return (uint32_t)actualRate;
}

void EpiqDriver_SetSampleRate(EpiqDriverHandle handle, uint32_t tunerIndex, uint32_t rate) {
    std::cout << "EpiqDriver_SetSampleRate called with tunerIndex: " << tunerIndex << " and rate: " << rate << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    // API doesn't let us set SR by itself, have to set BW too. So let's use the currently configured BW
    uint32_t oldSR, oldBW;
    uint32_t retVal = getProgrammedSRandBW(driverdata->cardNum, tunerIndexHdl, &oldSR, &oldBW);
    if(retVal != 0){
        std::cout << "Error: Fetching sample rate and bandwidth failed with error code " << retVal << std::endl;
        return;
    }
    retVal = skiq_write_rx_sample_rate_and_bandwidth(driverdata->cardNum, tunerIndexHdl, rate, oldBW);
    if(retVal != 0){
        std::cout << "Error: Setting sample rate failed with error code " << retVal << std::endl;
    }
}

uint32_t EpiqDriver_GetRxBandwidth(EpiqDriverHandle handle, uint32_t tunerIndex) {
    std::cout << "EpiqDriver_GetRxBandwidth called with tunerIndex: " << tunerIndex << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    double sr;
    uint32_t bw;
    uint32_t retVal = getActualSRandBW(driverdata->cardNum, tunerIndexHdl, &sr, &bw);
    if(retVal != 0){
        std::cout << "Error: Fetching sample rate and bandwidth failed with error code " << retVal << std::endl;
    }
    return bw;
}

void EpiqDriver_SetRxBandwidth(EpiqDriverHandle handle, uint32_t tunerIndex, uint32_t bandwidth) {
    std::cout << "EpiqDriver_SetRxBandwidth called with tunerIndex: " << tunerIndex << " and bandwidth: " << bandwidth << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    // API doesn't let us set BW by itself, have to set SR too. So let's use the currently configured SR
    uint32_t oldSR, oldBW;
    uint32_t retVal = getProgrammedSRandBW(driverdata->cardNum, tunerIndexHdl, &oldSR, &oldBW);
    if(retVal != 0){
        std::cout << "Error: Fetching sample rate and bandwidth failed with error code " << retVal << std::endl;
        return;
    }
    retVal = skiq_write_rx_sample_rate_and_bandwidth(driverdata->cardNum, tunerIndexHdl, oldSR, bandwidth);
    if(retVal != 0){
        std::cout << "Error: Setting sample rate failed with error code " << retVal << std::endl;
    }
}

uint32_t EpiqDriver_GetGain(EpiqDriverHandle handle, uint32_t tunerIndex) {
    std::cout << "EpiqDriver_GetGain called with tunerIndex: " << tunerIndex << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    uint8_t gainIdxMin, gainIdxMax, gainIdx; 
    skiq_read_rx_gain_index_range (driverdata->cardNum, tunerIndexHdl, &gainIdxMin, &gainIdxMax);
    std::cout << "Gain range: " << static_cast<uint32_t>(gainIdxMin) << " - " << static_cast<uint32_t>(gainIdxMax) << std::endl;
    skiq_read_rx_gain(driverdata->cardNum, tunerIndexHdl, &gainIdx);
    std::cout << "Reported gain index: " << static_cast<uint32_t>(gainIdx) << std::endl;
    return getGainDBFromGainIdx(gainIdx, gainIdxMin, gainIdxMax);    
}

void EpiqDriver_SetGain(EpiqDriverHandle handle, uint32_t tunerIndex, uint32_t gain) {
    std::cout << "EpiqDriver_SetGain called with tunerIndex: " << tunerIndex << " and gain: " << gain << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    uint8_t gainIdxMin, gainIdxMax, gainIdx; 
    skiq_read_rx_gain_index_range (driverdata->cardNum, tunerIndexHdl, &gainIdxMin, &gainIdxMax);
    gainIdx = getGainIdxFromGainDB(gain, gainIdxMin, gainIdxMax);
    std::cout << "Derived gain index: " << static_cast<uint32_t>(gainIdx) << std::endl;
    uint32_t retVal = skiq_write_rx_gain(driverdata->cardNum, tunerIndexHdl, gainIdx);
    if(retVal != 0){
        std::cout << "Error: Setting gain failed with error code " << retVal << std::endl;
    }
}

bool EpiqDriver_GetDcBias(EpiqDriverHandle handle, uint32_t tunerIndex) {
    std::cout << "EpiqDriver_GetDcBias called with tunerIndex: " << tunerIndex << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    bool isDCBiasEnabled;
    uint32_t retVal = skiq_read_rx_dc_offset_corr(driverdata->cardNum, tunerIndexHdl, &isDCBiasEnabled);
    if(retVal != 0){
        std::cout << "Error: Fetching DC Bias state failed with error code " << retVal << std::endl;
    }
    return isDCBiasEnabled;
}

void EpiqDriver_SetDcBias(EpiqDriverHandle handle, uint32_t tunerIndex, bool dcBias) {
    std::cout << "EpiqDriver_SetDcBias called with tunerIndex: " << tunerIndex << " and dcBias: " << dcBias << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    uint32_t retVal = skiq_write_rx_dc_offset_corr(driverdata->cardNum, tunerIndexHdl, dcBias);
    if(retVal != 0){
        std::cout << "Error: Setting DC Bias state failed with error code " << retVal << std::endl;
    }
}

bool EpiqDriver_GetAgc(EpiqDriverHandle handle, uint32_t tunerIndex) {
    std::cout << "EpiqDriver_GetAgc called with tunerIndex: " << tunerIndex << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    skiq_rx_gain_t gainMode; 
    int32_t retVal = skiq_read_rx_gain_mode (driverdata->cardNum, tunerIndexHdl, &gainMode);
    if(retVal != 0){
        std::cout << "Error: Fetching gain mode failed with error code " << retVal << std::endl;
    }
    return gainMode == skiq_rx_gain_auto;
}

void EpiqDriver_SetAgc(EpiqDriverHandle handle, uint32_t tunerIndex, bool agc) {
    std::cout << "EpiqDriver_SetAgc called with tunerIndex: " << tunerIndex << " and agc: " << agc << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    skiq_rx_gain_t gainMode = agc ? skiq_rx_gain_auto : skiq_rx_gain_manual; 
    int32_t retVal = skiq_write_rx_gain_mode (driverdata->cardNum, tunerIndexHdl, gainMode);
    if(retVal != 0){
        std::cout << "Error: Setting gain mode failed with error code " << retVal << std::endl;
    }
}

void EpiqDriver_StartRxStream(EpiqDriverHandle handle, uint32_t tunerIndex) {
    std::cout << "EpiqDriver_StartRxStream called with tunerIndex: " << tunerIndex << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);    
    uint32_t retVal = skiq_start_rx_streaming(driverdata->cardNum, tunerIndexHdl);
    if(retVal != 0){
        std::cout << "Error: Starting RX stream on tuner " << tunerIndex << " failed with error code " << retVal << std::endl;
    }
}

void EpiqDriver_StopRxStream(EpiqDriverHandle handle, uint32_t tunerIndex) {
    std::cout << "EpiqDriver_StopRxStream called with tunerIndex: " << tunerIndex << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_rx_hdl_t tunerIndexHdl = static_cast<skiq_rx_hdl_t>(tunerIndex);
    uint32_t retVal = skiq_stop_rx_streaming(driverdata->cardNum, tunerIndexHdl);
    if(retVal != 0){
        std::cout << "Error: Stopping RX stream on tuner " << tunerIndex << " failed with error code " << retVal << std::endl;
    }
}

float EpiqDriver_GetLatitude(EpiqDriverHandle handle) {
    std::cout << "EpiqDriver_GetLatitude called" << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    return driverdata->gpsInfo.latitude;
}

float EpiqDriver_GetLongitude(EpiqDriverHandle handle) {
    std::cout << "EpiqDriver_GetLongitude called" << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;  
    return driverdata->gpsInfo.longitude;
}

float EpiqDriver_GetElevation(EpiqDriverHandle handle) {
    std::cout << "EpiqDriver_GetElevation called" << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;  
    return driverdata->gpsInfo.altitude;
}

uint32_t EpiqDriver_GetTxFrequency(EpiqDriverHandle handle, uint32_t tunerIndex) {
    std::cout << "EpiqDriver_GetTxFrequency called with tunerIndex: " << tunerIndex << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_tx_hdl_t tunerIndexHdl = static_cast<skiq_tx_hdl_t>(tunerIndex);
    uint64_t programmedFreq;
    double actualFreq;
    int32_t retVal = skiq_read_tx_LO_freq(driverdata->cardNum, tunerIndexHdl, &programmedFreq, &actualFreq);
    return (uint32_t)actualFreq;
}

void EpiqDriver_SetTxFrequency(EpiqDriverHandle handle, uint32_t tunerIndex, uint32_t freq) {
    std::cout << "EpiqDriver_SetTxFrequency called with tunerIndex: " << tunerIndex << " and freq: " << freq << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_tx_hdl_t tunerIndexHdl = static_cast<skiq_tx_hdl_t>(tunerIndex);
    uint32_t retVal = skiq_write_tx_LO_freq(driverdata->cardNum, tunerIndexHdl, (uint64_t) freq);
}

void EpiqDriver_StartTxStream(EpiqDriverHandle handle, uint32_t tunerIndex) {
    std::cout << "EpiqDriver_StartTxStream called with tunerIndex: " << tunerIndex << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_tx_hdl_t tunerIndexHdl = static_cast<skiq_tx_hdl_t>(tunerIndex);
    uint32_t retVal = skiq_start_tx_streaming(driverdata->cardNum, tunerIndexHdl);
    if(retVal != 0){
        std::cout << "Error: Starting TX stream on tuner " << tunerIndex << " failed with error code " << retVal << std::endl;
    }
}

void EpiqDriver_StopTxStream(EpiqDriverHandle handle, uint32_t tunerIndex) {
    std::cout << "EpiqDriver_StopTxStream called with tunerIndex: " << tunerIndex << std::endl;
    EpiqDriverData *driverdata = (EpiqDriverData*)handle;
    skiq_tx_hdl_t tunerIndexHdl = static_cast<skiq_tx_hdl_t>(tunerIndex);
    uint32_t retVal = skiq_stop_tx_streaming(driverdata->cardNum, tunerIndexHdl);
    if(retVal != 0){
        std::cout << "Error: Stopping TX stream on tuner " << tunerIndex << " failed with error code " << retVal << std::endl;
    }
}
#ifndef CIRCULAR_BUFFER_HPP
#define CIRCULAR_BUFFER_HPP

#include <vector>
#include <cstdint> // For int16_t
#include <cstddef> // For size_t
#include <array>   // For std::array
#include <stdexcept> // For potential exceptions
#include <cstring> // For memcpy <<-- Added for memcpy
#include <numeric> // For std::iota (in example)
#include <iostream>// For example output
#include <cassert> // For assert (in example)

#include "x40_constants.hpp"


constexpr size_t DEFAULT_BUFFER_CAPACITY = 256; // Max storage = CAPACITY - 1 = 255 blocks

// This isn't really a generic buffer because we're doing some data padding optimizations
// specifically for the X40 driver.
class X40DataBuffer {
private:
    size_t slotCapacity; // Total number of slots allocated (e.g., 256)
    std::vector<std::array<int16_t, PKT_LEN>> buf; 
    size_t readPtr;  // Index of the next block to read 
    size_t writePtr; // Index of the next block to write 

public:
    /**
     * @brief Constructor to initialize the circular buffer.
     * @param slotCapacity The total number of slots (BUFFER_CAPACITY). Note: Usable slotCapacity is slotCapacity - 1.
     */
    explicit X40DataBuffer(size_t slotCapacity = DEFAULT_BUFFER_CAPACITY)
        : slotCapacity(slotCapacity),
          buf(slotCapacity), // Allocate the vector
          readPtr(0),
          writePtr(0) 
    {
        if (slotCapacity < 2) {
            throw std::invalid_argument("Capacity must be at least 2 for this implementation.");
        }
        for(size_t i = 0; i < slotCapacity; ++i) {
            buf[i].fill(0); // Initialize each block to zero
        }
    }


    /**
     * @brief Inserts a data block into the buffer.
     * If the buffer is full, it will not insert the data and return false.
     * @param data Pointer to the data to be inserted.
     * @param dataLen Length of the data to be inserted (default is PKT_LEN).
     * @return true on success, false if the buffer is full.
     */
    bool insert(const int16_t *data, size_t dataLen = PKT_LEN) {
        // TODO: figure out an efficient way to do data padding
        // in the driver itself
        if (dataLen > PKT_LEN) {
            dataLen = PKT_LEN;
        }
        if (isFull()) {
            return false; // Indicate buffer is full
        }

        std::copy(data, data + dataLen, buf[writePtr].begin());

        // Advance the write pointer, wrapping around using modulo
        writePtr = (writePtr + 1) % slotCapacity; 

        return true; // Indicate success
    }

    /**
     * @brief Reads a data block from the buffer using memcpy.
     * Copies the data from the read position into the destination 'data' struct.
     * Fails if the buffer is empty.
     * @param data A data_block_t struct where the read data will be stored.
     * @return true on success, false if the buffer is empty.
     */
    bool read(std::array<int16_t, PKT_LEN>& dest) { // Pass by non-const reference to store result
        if (isEmpty()) {
            return false; // Indicate buffer is empty
        }
        std::copy(buf[readPtr].begin(), buf[readPtr].end(), dest.begin());
        
        // Advance the read pointer, wrapping around using modulo
        readPtr = (readPtr + 1) % slotCapacity; 

        return true; // Indicate success
    }

    /**
     * @brief Gets the current read pointer index.
     * @return The current read index.
     */
    size_t getReadPtr() const {
        return readPtr;
    }

    /**
     * @brief Gets the current write pointer index.
     * @return The current write index.
     */
    size_t getWritePtr() const {
        return writePtr;
    }

    /**
     * @brief Checks if the read and write pointers overlap (indicates empty buffer).
     * @return true if read_ptr == write_ptr (buffer is empty), false otherwise.
     */
    bool checkOverlap() const {
        // In waste-a-slot, overlap means empty
        return isEmpty();
    }

    /**
     * @brief Checks if the buffer is full.
     * Uses the "waste-a-slot" logic: (write_ptr + 1) % slotCapacity == read_ptr.
     * @return true if the buffer is full, false otherwise.
     */
    bool isFull() const {
        // Using the waste-a-slot strategy         
        return (writePtr + 1) % slotCapacity == readPtr;
    }

    /**
     * @brief Checks if the buffer is empty.
     * Uses the logic: read_ptr == write_ptr.
     * @return true if the buffer is empty, false otherwise.
     */
    bool isEmpty() const {
        // In waste-a-slot, empty means read_ptr == write_ptr
        return readPtr == writePtr;
    }

    /**
     * @brief Gets the current number of elements stored in the buffer.
     * @return The number of elements currently in the buffer.
     */
    size_t size() const { 
        if (writePtr >= readPtr) {
            return writePtr - readPtr;
        } else {
            // Handles wrap-around case
            return slotCapacity - readPtr + writePtr;
        }
    }

    /**
     * @brief Gets the maximum number of elements the buffer can hold (usable slotCapacity).
     * @return The usable slotCapacity (total slots - 1).
     */
    size_t getCapacity() const {
        // Effective slotCapacity due to waste-a-slot 
        return slotCapacity - 1;
    }
};

#endif


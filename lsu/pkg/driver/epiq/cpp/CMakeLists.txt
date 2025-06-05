cmake_minimum_required(VERSION 3.10)
project(epiq_driver CXX)

set(CMAKE_CXX_STANDARD 11) 

# Replace these paths with the actual locations of your SDK.
# These work on B015 (.89 unit)
set(SIDEKIQX40_SDK_ROOT "/home/sidekiq/sidekiq_sdk_current")
set(SIDEKIQX40_SDK_SUPPORT "${SIDEKIQX40_SDK_ROOT}/lib/support/msiq-x40")

set(
    SIDEKIQX40_SDK_INCLUDE_DIR 
    "${SIDEKIQX40_SDK_ROOT}/sidekiq_core/inc"
    "${SIDEKIQX40_SDK_ROOT}/arg_parser/inc"
)

set(
    SIDEKIQX40_SDK_LIBRARY_DIR 
    "${SIDEKIQX40_SDK_ROOT}/lib"
    "${SIDEKIQX40_SDK_SUPPORT}/usr/lib/epiq"
)

include_directories(${SIDEKIQX40_SDK_INCLUDE_DIR})
link_directories(${SIDEKIQX40_SDK_LIBRARY_DIR})



# -----------------------------------------------------------------------------
# X40 SDK Additional Config
# -----------------------------------------------------------------------------
set(custom_pkg_config_path "${SIDEKIQX40_SDK_SUPPORT}/usr/lib/epiq/pkgconfig")

# Construct the new PKG_CONFIG_PATH by appending the custom path
if(NOT "${current_pkg_config_path}" STREQUAL "")
    set(ENV{PKG_CONFIG_PATH} "${current_pkg_config_path}:${custom_pkg_config_path}")
else()
    set(ENV{PKG_CONFIG_PATH} "${custom_pkg_config_path}")
endif()

# Find the PkgConfig module
find_package(PkgConfig REQUIRED)

# Use pkg_check_modules to find the gRPC++ and protobuf libraries
pkg_check_modules(gRPC++ REQUIRED grpc++)
pkg_check_modules(Protobuf REQUIRED protobuf)

message(STATUS "gRPC++_LIBRARIES: ${gRPC++_LIBRARIES}")
message(STATUS "++Protobuf_LIBRARIES: ${Protobuf_LIBRARIES}")
#------------------------------------------------------------------------------
# libraries
#------------------------------------------------------------------------------
set(
    EXTRA_LIBRARIES
    "usb-1.0"
    "glib-2.0"
    "gpiod" 
    "stdc++"
    "tirpc"
    "pthread" "rt" "m"
    "gps"
)

set(
    SIDEKIQX40_LIBRARIES 
    "sidekiq__msiq-x40"
    "${gRPC++_LIBRARIES}"
    "${Protobuf_LIBRARIES}"
    "${EXTRA_LIBRARIES}"
    
)
# -----------------------------------------------------------------------------
# Library sources
# -----------------------------------------------------------------------------
set(DRIVER_SOURCES
    epiq_driver.cc
    gps_utils.cc
)

# -----------------------------------------------------------------------------
# Create the library
# -----------------------------------------------------------------------------
add_library(epiq_driver SHARED ${DRIVER_SOURCES})

# -----------------------------------------------------------------------------
# Link against the Epiq Matchstiq libraries
# -----------------------------------------------------------------------------
target_link_libraries(epiq_driver PUBLIC ${SIDEKIQX40_LIBRARIES})

# -----------------------------------------------------------------------------
# Optional: Add installation rules
# -----------------------------------------------------------------------------
# install(TARGETS epiq_driver DESTINATION bin)
# install(DIRECTORY include DESTINATION include) # If you have public headers


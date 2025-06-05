cmake_minimum_required(VERSION 3.10)
project(b210_driver LANGUAGES CXX CUDA)

find_package(UHD REQUIRED)
find_package(CUDAToolkit REQUIRED)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED True)
set(CMAKE_CUDA_STANDARD 17) 
set(CMAKE_CUDA_STANDARD_REQUIRED True)
set(CMAKE_CUDA_ARCHITECTURES 70 75 80 86) # Adjust based on your GPU architecture

include_directories(${UHD_INCLUDE_DIRS})
add_library(b210_driver STATIC b210_driver.cu)
target_link_libraries(b210_driver PUBLIC
  ${UHD_LIBRARIES}
  CUDA::cufft
  CUDA::cudart
) 

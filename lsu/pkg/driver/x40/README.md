# Epiq Matchstiq X40 Driver

This project builds a driver in the form of a shared library for interacting with the Epiq Sidekiq X40 SDR.

The project layout is important...we keep the CPP files in the `cpp` folder because if they're in the same folder as the `epiq_driver.go` file, then when we try to build the command-line client, it will try to build the CPP files and that fails epically.


## Build Steps
The best way to build this library is on an X40 device with the SDK installed. You'll also need these prerequisistes:
- GCC (likely already installed)
- CMake (likely already installed)
- libgps26 (likely already installed)
- libgps-dev (`sudo apt install libgps-dev`)

### CMake Tweaks
CMake needs to be able to find the sidekiq SDK and its bundled dependencies, so make sure you edit the `SIDEKIQX40_SDK_ROOT` variable in the `CMakeLists.txt` file to point to the root folder of the SDK. By default, it is:
```
set(SIDEKIQX40_SDK_ROOT "/home/sidekiq/sidekiq_sdk_current")
```

*Note: you will notice some extra packageconfig stuff going on in the section under the comment "X40 SDK Additional Config"; this is because the X40 library has additional dependencies which in turn have about 100 other dependencies and all those libraries are bundled into the SDK folder and made findable via packageconfig. You can look at how the sidekiq SDK makefiles link them if you're curious.*

### CMake Build
```bash
PRJ_ROOT=/home/astarion/lsu # wherever you cloned the project

cd $PRJ_ROOT/pkg/driver/epiq
mkdir build
cd build

cmake ../cpp 
make
```
This will generate a `libepiq_driver.so` file under `$PRJ_ROOT/pkg/driver/epiq`.

## CLI
You can build the top-level CLI as normal. However, in order to run it with the X40 driver, you need to make the `libepiq_driver.so` file available on the LD_LIBRARY_PATH. So assuming the project is located at `/home/astarion/lsu`, do the following:

```bash
LD_LIBRARY_PATH=$LD_LIBRARY_PATH:/home/astarion/lsu/pkg/driver/epiq/build
export LD_LIBRARY_PATH
```

and then run the CLI.


#include <iostream>

#include <sidekiq_api.h>
#include <sidekiq_params.h>

#include "epiq_driver.h"
#include <unistd.h>


int main() {
  std::cout << "Hello from the Matchstiq X40 reader application!" << std::endl;
  EpiqDriverHandle hdl = EpiqDriver_Init("Hallo");
  for (int i = 0; i < 10; i++){
    std::cout << "ITER " << i << std::endl;
    sleep(0.01);
  }
  EpiqDriver_Close(hdl);
  return 0;
}
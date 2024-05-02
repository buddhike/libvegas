#include "vegas.h"

#include <string>
#include <vector>
#include <map>

#ifndef PRODUCER_HPP_
#define PRODUCER_HPP_

namespace vegas
{
    
class Producer {
   public:
    Producer(std::string_view streamName, VegasProducerConfig config);
    void send(std::string_view partitionKey, std::string_view data);

   private:
   VegasProducerConfig config;
};

} // namespace vegas

#endif
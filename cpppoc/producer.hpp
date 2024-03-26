#include <string>
#include <vector>
#include <map>

#ifndef PRODUCER_HPP_
#define PRODUCER_HPP_

class Producer {
   public:
    Producer();
    void send(std::string_view partition_key, std::vector<char>& data);

   private:
};

#endif
#include "vegas.h"
#include "producer.hpp"

#include <map>
#include <mutex>
#include <shared_mutex>
#include <string>

#ifdef __cplusplus
extern "C" {
#endif

static std::map<size_t, std::unique_ptr<vegas::Producer>> producers;
static std::shared_mutex producers_lock;

struct VegasProducerConfig vegas_create_default_producer_config() {
    struct VegasProducerConfig c = {
        .batch_size_record_count = 100,
        .batch_timeout_ms = 100
    };

    return c;
};

int vegas_create_producer(const char* stream_name, struct VegasProducerConfig config) {
    std::unique_lock lck(producers_lock);
    auto id = producers.size();
    producers[id + 1] = std::make_unique<vegas::Producer>(std::string_view(stream_name), config);
    return id;
}

int vegas_send(const int producer_id, const char* partition_key, const char* data, const int data_length) {
    std::shared_lock lck(producers_lock);
    if (auto iter = producers.find(producer_id); iter != producers.end()) {
       iter->second->send(std::string_view(partition_key), std::string_view(data, data_length)); 
    }
    return 0;
}

#ifdef __cplusplus
}
#endif

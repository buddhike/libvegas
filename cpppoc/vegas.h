#ifndef VEGAS_H_
#define VEGAS_H_

#ifdef __cplusplus
extern "C" {
#endif

struct VegasProducerConfig {
    int batch_size_record_count;
    int batch_timeout_ms;
};

struct VegasProducerConfig vegas_create_default_producer_config();
int vegas_create_producer(const char* stream_name, struct VegasProducerConfig config);
int vegas_send(const int producer_id, const char* partition_key, const char* data, const int data_length);

#ifdef __cplusplus
}
#endif

#endif
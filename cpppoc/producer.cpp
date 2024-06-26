#include "vegas.h"
#include "producer.hpp"
#include <aws/core/Aws.h>
#include <aws/kinesis/KinesisClient.h>
#include <aws/kinesis/model/DescribeStreamRequest.h>
#include <aws/kinesis/model/DescribeStreamConsumerResult.h>

vegas::Producer::Producer(std::string_view streamName, VegasProducerConfig config) {
    this->config = config;
}

void vegas::Producer::send(std::string_view partitionKey, std::string_view data) {
}

// int main() {
//     Aws::SDKOptions options;
//     Aws::InitAPI(options);
//     {
//         Aws::Client::ClientConfiguration config;
//         config.region = Aws::Region::AP_SOUTHEAST_2;
//         auto client {Aws::Kinesis::KinesisClient(config)};
//         auto req = Aws::Kinesis::Model::DescribeStreamRequest();
//         req.SetStreamName("test");
//         auto res = client.DescribeStream(req);
//         std::cout << res.GetResult().GetStreamDescription().GetStreamARN() << "\n";
//     }
//     Aws::ShutdownAPI(options);
//     return 0;
// }

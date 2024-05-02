#include <stdio.h>
#include <string.h>
#include "vegas.h"

int main() {
    int h = 0;
    char* err;
    char* msg = "hello from c";
    struct VegasProducerConfig c = vegas_create_default_producer_config();
    int p = vegas_create_producer("test", c);
    p = vegas_send(p, "c", msg, strlen(msg));
    return 0;
}
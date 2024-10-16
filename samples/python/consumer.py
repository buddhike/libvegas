from pebble import Consumer 

def process_record(r):
    print(str(r.partitionKey) + ":" + r.data.decode("utf-8"))

stream = "test"
efo = "arn:aws:kinesis:ap-southeast-2:767660010185:stream/test/consumer/python-consumer:1686199962"
c = Consumer(stream, efo, process_record)

input("Consumer is running. Press ctrl+c to stop\n")

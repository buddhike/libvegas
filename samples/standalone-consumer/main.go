package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/service/kinesis/types"
	"github.com/buddhike/pebble/consumer"
)

var (
	streamName        string
	streamConsumerArn string
)

func init() {
	flag.StringVar(&streamName, "stream-name", "", "Stream name")
	flag.StringVar(&streamConsumerArn, "stream-consumer-arn", "", "EFO consumer ARN")
}

func main() {
	flag.Parse()

	processFn := func(record types.Record) {
		fmt.Printf("Processing record: %s\n", *record.PartitionKey)
	}

	c := consumer.MustNewStandaloneConsumer("my-consumer", streamName, streamConsumerArn, processFn)
	ctx, cancel := context.WithCancel(context.Background())

	// Start the consumer
	err := c.Start(ctx)
	if err != nil {
		panic(err)
	}

	notifyChan := make(chan os.Signal, 1)
	signal.Notify(notifyChan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-notifyChan
		cancel()
	}()

	// Wait for the consumer to finish
	c.WaitForShutdownCompletion()
}

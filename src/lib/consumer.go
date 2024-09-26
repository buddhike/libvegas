package lib

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/buddhike/libvegas/lib/pb"
)

type RecordProcessor func(*pb.UserRecord) error

type Consumer struct {
	sm   *subscriptionManager
	done chan struct{}
}

func (c *Consumer) Done() <-chan struct{} {
	return c.done
}

func NewConsumer(streamName, consumerARN string, p RecordProcessor) (*Consumer, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, err
	}
	kc := kinesis.NewFromConfig(cfg)
	d, err := kc.DescribeStream(context.TODO(), &kinesis.DescribeStreamInput{
		StreamName: &streamName,
	})
	if err != nil {
		return nil, err
	}
	sm := &subscriptionManager{
		streamARN:   d.StreamDescription.StreamARN,
		consumerARN: &consumerARN,
		kc:          kc,
		p:           p,
		rfs:         []recordFilter{newInvalidMappingFilter()},
		urfs:        []userRecordFilter{},
	}
	go sm.Start()
	return &Consumer{
		sm:   sm,
		done: make(chan struct{}),
	}, nil
}

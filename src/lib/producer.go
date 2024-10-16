package lib

import (
	"context"
	"crypto/md5"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/buddhike/pebble/lib/pb"
)

type Producer struct {
	stream   string
	shardMap *shardMap
	done     chan struct{}
}

func (p *Producer) Send(ctx context.Context, partitionKey string, data []byte) error {
	h := md5.New()
	h.Write([]byte(partitionKey))
	h.Write(data)
	rid := h.Sum([]byte{})
	return p.SendWithRecordID(ctx, partitionKey, rid[:], data)
}

func (p *Producer) SendWithRecordID(ctx context.Context, partitionKey string, recordID, data []byte) error {
	var wr *writeRequest
forever:
	for {
		rr := &resolveRequest{
			partitionKey: partitionKey,
			response:     make(chan *shardWriter),
		}
		p.shardMap.request <- rr
		m := <-rr.response
		wr = &writeRequest{
			UserRecord: &pb.UserRecord{
				PartitionKey: partitionKey,
				RecordID:     recordID,
				Data:         data,
			},
			Response: make(chan *writeResponse, 1),
		}
		select {
		case m.input <- wr:
			break forever
		case <-m.close:
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	select {
	case wrr := <-wr.Response:
		if wrr != nil {
			return wrr.Err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Producer) Done() <-chan struct{} {
	return p.done
}

type ProducerConfig struct {
	BufferSize     int
	BatchSize      int
	BatchTimeoutMS int
}

var DefaultProducerConfig ProducerConfig = ProducerConfig{
	BufferSize:     100,
	BatchSize:      100,
	BatchTimeoutMS: 100,
}

func NewProducer(streamName string, pc ProducerConfig) (*Producer, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, err
	}
	kc := kinesis.NewFromConfig(cfg)
	s, err := kc.DescribeStream(context.TODO(), &kinesis.DescribeStreamInput{
		StreamName: &streamName,
	})
	if err != nil {
		return nil, err
	}
	sm := newShardMap(*s.StreamDescription.StreamARN, pc.BufferSize, pc.BatchSize, pc.BatchTimeoutMS, kc)
	go sm.Start()
	return &Producer{
		stream:   streamName,
		shardMap: sm,
	}, nil
}

type producerClient interface {
	PutRecord(ctx context.Context, params *kinesis.PutRecordInput, optFns ...func(*kinesis.Options)) (*kinesis.PutRecordOutput, error)
	ListShards(ctx context.Context, params *kinesis.ListShardsInput, optFns ...func(*kinesis.Options)) (*kinesis.ListShardsOutput, error)
}

package pebble

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kinesis/types"
)

type subscriptionManager struct {
	streamARN   *string
	consumerARN *string
	kc          consumerClient
	p           RecordProcessor
	rfs         []recordFilter
	urfs        []userRecordFilter
	done        chan struct{}
}

type consumerClient interface {
	SubscribeToShard(ctx context.Context, params *kinesis.SubscribeToShardInput, optFns ...func(*kinesis.Options)) (*kinesis.SubscribeToShardOutput, error)
	ListShards(ctx context.Context, params *kinesis.ListShardsInput, optFns ...func(*kinesis.Options)) (*kinesis.ListShardsOutput, error)
}

func (m *subscriptionManager) Start() {
	l, err := m.kc.ListShards(context.TODO(), &kinesis.ListShardsInput{
		StreamARN: m.streamARN,
	})
	if err != nil {
		panic(err)
	}
	p := make(map[string]bool)
	for _, s := range l.Shards {
		p[*s.ShardId] = true
	}
	children := make(chan []types.ChildShard)
	for _, s := range l.Shards {
		if (s.ParentShardId == nil && s.AdjacentParentShardId == nil) ||
			(s.ParentShardId != nil && !p[*s.ParentShardId]) &&
				s.AdjacentParentShardId == nil ||
			(s.AdjacentParentShardId != nil && !p[*s.AdjacentParentShardId]) {
			r := &shardReader{
				consumerARN: m.consumerARN,
				c:           &shardReaderContext{shardID: *s.ShardId},
				kc:          m.kc,
				p:           m.p,
				rfs:         m.rfs,
				urfs:        m.urfs,
				done:        m.done,
				children:    children,
			}
			go r.Start()
		}
	}
	for {
		select {
		case ch := <-children:
			for _, c := range ch {
				r := &shardReader{
					consumerARN: m.consumerARN,
					c:           &shardReaderContext{shardID: *c.ShardId},
					kc:          m.kc,
					p:           m.p,
					rfs:         m.rfs,
					urfs:        m.urfs,
					done:        m.done,
					children:    children,
				}
				go r.Start()
			}
		case <-m.done:
			return
		}
	}
}

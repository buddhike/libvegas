package lib

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kinesis/types"
	"github.com/buddhike/pebble/lib/pb"
	"google.golang.org/protobuf/proto"
)

type shardReader struct {
	consumerARN *string
	kc          consumerClient
	p           RecordProcessor
	c           *shardReaderContext
	rfs         []recordFilter
	urfs        []userRecordFilter
	done        chan struct{}
	children    chan<- []types.ChildShard
}

func (r *shardReader) Start() {
	s, err := r.kc.SubscribeToShard(context.TODO(), &kinesis.SubscribeToShardInput{
		ShardId:     &r.c.shardID,
		ConsumerARN: r.consumerARN,
		StartingPosition: &types.StartingPosition{
			Type: types.ShardIteratorTypeLatest,
		},
	})
	if err != nil {
		panic(err)
	}
	for {
		c, seq := r.consume(s.GetStream().Events())
		if c {
			s, err = r.kc.SubscribeToShard(context.TODO(), &kinesis.SubscribeToShardInput{
				ShardId:     &r.c.shardID,
				ConsumerARN: r.consumerARN,
				StartingPosition: &types.StartingPosition{
					Type:           types.ShardIteratorTypeAtSequenceNumber,
					SequenceNumber: seq,
				},
			})
			if err != nil {
				panic(err)
			}
		}
	}
}

func (r *shardReader) consume(s <-chan types.SubscribeToShardEventStream) (bool, *string) {
	var continuationSeq *string
	for {
		select {
		case i, ok := <-s:
			if !ok {
				return true, continuationSeq
			}
			if te, ok := i.(*types.SubscribeToShardEventStreamMemberSubscribeToShardEvent); ok {
				value := te.Value
				if len(value.ChildShards) > 0 {
					r.children <- value.ChildShards
					return false, nil
				}
				for _, kr := range value.Records {
					rcd := pb.Record{}
					err := proto.Unmarshal(kr.Data, &rcd)
					if err != nil {
						panic(err)
					}

					if r.shouldFilterRecord(&rcd) {
						continue
					}

					for _, ur := range rcd.UserRecords {
						if r.shouldFilterUserRecord(ur) {
							continue
						}
						err := r.p(ur)
						if err != nil {
							panic(err)
						}
					}
				}
				continuationSeq = value.ContinuationSequenceNumber
			}
		case <-r.done:
			return false, nil
		}
	}
}

func (r *shardReader) shouldFilterRecord(rcd *pb.Record) bool {
	for _, rf := range r.rfs {
		if rf.Apply(r.c, rcd) {
			return true
		}
	}
	return false
}

func (r *shardReader) shouldFilterUserRecord(rcd *pb.UserRecord) bool {
	for _, rf := range r.urfs {
		if rf.Apply(r.c, rcd) {
			return true
		}
	}
	return false
}

type recordFilter interface {
	Apply(*shardReaderContext, *pb.Record) bool
}

type userRecordFilter interface {
	Apply(*shardReaderContext, *pb.UserRecord) bool
}

type shardReaderContext struct {
	shardID string
}

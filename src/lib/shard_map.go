package lib

import (
	"context"
	"crypto/md5"
	"errors"
	"math/big"

	"github.com/aws/aws-sdk-go-v2/service/kinesis"
)

type shardMap struct {
	streamARN      *string
	version        int
	request        chan *resolveRequest
	done           chan struct{}
	invalidations  chan int
	kc             producerClient
	bufferSize     int
	batchSize      int
	batchTimeoutMS int
	entries        []shardMapEntry
}

type resolveRequest struct {
	partitionKey string
	response     chan *shardWriter
}

type shardMapEntry struct {
	start  big.Int
	end    big.Int
	writer *shardWriter
}

func (m *shardMap) Start() {
	m.build()
forever:
	for {
		select {
		case rr := <-m.request:
			w := m.resolve(rr.partitionKey)
			rr.response <- w
		case v := <-m.invalidations:
			if m.version == v {
				m.rebuild()
			}
		case <-m.done:
			break forever
		}
	}
}

func (m *shardMap) build() {
	m.version++
	m.entries = m.entries[:0]
	o, err := m.kc.ListShards(context.TODO(), &kinesis.ListShardsInput{
		StreamARN: m.streamARN,
	})
	if err != nil {
		panic(err)
	}

	p := make(map[string]bool)
	for _, s := range o.Shards {
		if s.ParentShardId != nil {
			p[*s.ParentShardId] = true
		}
		if s.AdjacentParentShardId != nil {
			p[*s.AdjacentParentShardId] = true
		}
	}

	for _, s := range o.Shards {
		if _, ok := p[*s.ShardId]; !ok {
			e := shardMapEntry{}
			e.start.SetString(*s.HashKeyRange.StartingHashKey, 10)
			e.end.SetString(*s.HashKeyRange.EndingHashKey, 10)
			e.writer = newShardWriter(m.streamARN, s.ShardId, s.HashKeyRange.StartingHashKey, m.bufferSize, m.batchSize, m.batchTimeoutMS, m.kc, m.done, m.invalidations)
			go e.writer.Start()
			m.entries = append(m.entries, e)
		}
	}
}

func (m *shardMap) resolve(partitionKey string) *shardWriter {
	h := md5.Sum([]byte(partitionKey))
	i := big.NewInt(0).SetBytes(h[0:])
	for _, en := range m.entries {
		s := i.Cmp(&en.start)
		e := i.Cmp(&en.end)
		if s >= 0 && e <= 0 {
			return en.writer
		}
	}
	panic(errors.New("not found"))
}

func (m *shardMap) rebuild() {
	requests := make([]*writeRequest, 0)
	for _, e := range m.entries {
		r := make(chan []*writeRequest)
		e.writer.drain <- r
		rs := <-r
		requests = append(requests, rs...)
	}
	m.build()
	for _, r := range requests {
		w := m.resolve(r.UserRecord.PartitionKey)
		w.input <- r
	}
}

func newShardMap(streamARN string, bufferSize, batchSize, batchTimeoutMS int, kc producerClient) *shardMap {
	return &shardMap{
		streamARN:      &streamARN,
		request:        make(chan *resolveRequest),
		done:           make(chan struct{}),
		invalidations:  make(chan int),
		kc:             kc,
		bufferSize:     bufferSize,
		batchSize:      batchSize,
		batchTimeoutMS: batchTimeoutMS,
	}
}

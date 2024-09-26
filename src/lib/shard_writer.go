package lib

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/buddhike/libvegas/lib/pb"
	"google.golang.org/protobuf/proto"
)

type shardWriter struct {
	shardID       *string                   // Shard ID written by this writer
	input         chan *writeRequest        // Receive messages to be written out
	drain         chan chan []*writeRequest // Signaled by ShardMap to collect outstanding writeRequests in this shardWriter's buffer
	close         chan struct{}             // Closed when shardWriter is no longer accepting writeRequests
	done          chan struct{}             // Closed when it's time to abort (e.g. SIGTERM)
	invalidations chan int                  // Used to notify shard map invalidations detected by this shardWriter
	kinesisClient producerClient            //
	batchSize     int                       //
	batchTimeout  int                       //
	partitionKey  *string                   //
	streamARN     *string                   //
	version       int                       // Shard writer is transient. Used to identify which iteration of shardMap prep created this shardWriter
}

type writeRequest struct {
	UserRecord *pb.UserRecord
	Response   chan *writeResponse
}

type writeResponse struct {
	Err error
}

func (w *shardWriter) Start() {
	batch := make([]*writeRequest, 0)
	send := true
	var sn *string
	batchTimeout := time.Millisecond * time.Duration(w.batchTimeout)
	t := time.NewTimer(batchTimeout)
forever:
	for {
		select {
		case wr, ok := <-w.input:
			if ok {
				batch = append(batch, wr)
				if send && len(batch) >= w.batchSize {
					sn, send, batch = w.sendBatch(batch, sn)
				}
			}
		case <-t.C:
			if send && len(batch) > 0 {
				sn, send, batch = w.sendBatch(batch, sn)
			}
			if send {
				t.Reset(batchTimeout)
			}
		case r := <-w.drain:
			close(w.close)
			for a := range w.input {
				batch = append(batch, a)
			}
			r <- batch
			break forever
		case <-w.done:
			close(w.close)
			break forever
		}
	}
}

func (w *shardWriter) sendBatch(batch []*writeRequest, sequenceNumber *string) (*string, bool, []*writeRequest) {
	ignoredPartitionKey := aws.String("a")
	ur := make([]*pb.UserRecord, 0)
	c := w.batchSize
	if len(batch) < c {
		c = len(batch)
	}
	batch = batch[0:c]
	for _, i := range batch {
		ur = append(ur, i.UserRecord)
	}
	r := &pb.Record{
		ShardID:     *w.shardID,
		UserRecords: ur,
	}
	m, err := proto.Marshal(r)
	if err != nil {
		panic(err)
	}
	p := &kinesis.PutRecordInput{
		PartitionKey:              ignoredPartitionKey, // Ignored because we use ExplicitHashKey
		ExplicitHashKey:           w.partitionKey,
		StreamARN:                 w.streamARN,
		Data:                      m,
		SequenceNumberForOrdering: sequenceNumber,
	}

	o, err := w.kinesisClient.PutRecord(context.TODO(), p)
	if err != nil {
		for _, i := range batch {
			i.Response <- &writeResponse{
				Err: err,
			}
			close(i.Response)
		}
		return sequenceNumber, true, batch[c:]
	} else {
		if *o.ShardId == *w.shardID {
			for _, i := range batch {
				close(i.Response)
			}
			return o.SequenceNumber, true, batch[c:]
		} else {
			// Although the record is written to KDS
			// it's written to the wrong shard. We don't
			// notify Response channel here.
			// New shardWriter assigned to handle the remapped
			// partition will eventually report success or
			// failure.
			// The key motivation here is to not surface
			// shard split/merge details to user API.
			close(w.close)
			w.invalidations <- w.version
			return nil, false, batch
		}
	}
}

func newShardWriter(streamARN, shardID, partitionKey *string, bufferSize int, batchSize, batchTimeoutMS int, kinesisClient producerClient, done chan struct{}, invalidations chan int) *shardWriter {
	return &shardWriter{
		streamARN:     streamARN,
		partitionKey:  partitionKey,
		shardID:       shardID,
		input:         make(chan *writeRequest, bufferSize),
		close:         make(chan struct{}),
		drain:         make(chan chan []*writeRequest),
		invalidations: invalidations,
		done:          done,
		kinesisClient: kinesisClient,
		batchSize:     batchSize,
		batchTimeout:  batchTimeoutMS,
	}
}

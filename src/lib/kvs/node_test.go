package kvs

import (
	"math/rand"
	"testing"
	"time"

	"github.com/buddhike/pebble/lib/kvs/pb"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNode(t *testing.T) {
	z, err := zap.NewProduction()
	assert.NoError(t, err)
	logger := z.Sugar()

	n1 := newTestNode("n1", logger)
	n2 := newTestNode("n2", logger)
	n3 := newTestNode("n3", logger)
	n4 := newTestNode("n4", logger)
	n5 := newTestNode("n5", logger)
	n1.SetPeers([]Peer{n2, n3, n4, n5})
	n2.SetPeers([]Peer{n1, n3, n4, n5})
	n3.SetPeers([]Peer{n1, n2, n4, n5})
	n4.SetPeers([]Peer{n1, n2, n3, n5})
	n5.SetPeers([]Peer{n1, n2, n3, n4})

	go n1.Start()
	go n2.Start()
	go n3.Start()
	go n4.Start()
	go n5.Start()
	time.Sleep(time.Minute)
}

func newTestNode(id string, logger *zap.SugaredLogger) *Node {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	electionTimeout := rnd.Intn(100) + 100
	log := &inMemoryLog{
		entries: make([]*pb.Entry, 0),
	}
	state := &inMemoryMap{
		i: make(map[string]string),
	}
	stop := make(chan struct{})
	return NewNode(id, time.Millisecond*50, time.Millisecond*time.Duration(electionTimeout), log, state, stop, logger)
}

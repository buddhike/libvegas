package kvs

import (
	"testing"
	"time"

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
	log := &inMemoryLog{}
	state := &inMemoryMap{}
	stop := make(chan struct{})
	return NewNode(id, time.Second, time.Second*5, log, state, stop, logger)
}

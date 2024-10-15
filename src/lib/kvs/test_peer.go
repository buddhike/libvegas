package kvs

type testPeer struct {
	peerID string
	in     chan request
}

func (p *testPeer) id() string {
	return p.peerID
}

func (p *testPeer) input() chan<- request {
	return p.in
}

func newTestPeer(id string, in chan request) *testPeer {
	return &testPeer{
		peerID: id,
		in:     in,
	}
}

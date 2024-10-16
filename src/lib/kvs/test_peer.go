package kvs

type testPeer struct {
	peerID string
	in     chan Req
}

func (p *testPeer) id() string {
	return p.peerID
}

func (p *testPeer) input() chan<- Req {
	return p.in
}

func newTestPeer(id string, in chan Req) *testPeer {
	return &testPeer{
		peerID: id,
		in:     in,
	}
}

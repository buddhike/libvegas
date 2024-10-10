package lib

import (
	"time"

	"github.com/buddhike/libvegas/lib/pb"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type electionOutcome int
type state int

type request struct {
	msg       proto.Message
	responder responder
}

type responder interface {
	respond(proto.Message)
}

const (
	electionOutcomeLeader electionOutcome = iota
	electionOutcomeFollower
	electionOutcomeInconclusive
)

const (
	stateFollower = iota
	stateCandidate
	stateLeader
	stateExit
)

type Manager struct {
	electionTimeout time.Duration
	id              string
	logger          *zap.SugaredLogger
	// Requests to this manager
	request chan request
	// Channels to send requests to peers
	requestPeers []chan<- proto.Message
	// Channel to receive responses from peers
	peerResponses <-chan proto.Message
	term          int64
}

func (m *Manager) Start() {
	n := m.becomeFollower()
	for n != stateExit {
		switch n {
		case stateCandidate:
			m.becomeCandidate()
		case stateLeader:
			m.becomeLeader()
		case stateFollower:
			m.becomeFollower()
		}
	}
}

func (m *Manager) becomeFollower() state {
	voted := false
	for {
		select {
		case v := <-m.request:
			switch msg := v.msg.(type) {
			case *pb.HeartbeatRequest:
				if msg.Term >= m.term {
					m.term = msg.Term
				} else {
					res := pb.HeartbeatResponse{
						Term: m.term,
					}
					v.responder.respond(&res)
				}
			case *pb.VoteRequest:
				vote := !voted && msg.Term >= m.term
				res := pb.VoteResponse{
					Term: m.term,
					Yes:  vote,
				}
				voted = voted || vote
				v.responder.respond(&res)

			}
		case <-time.After(m.electionTimeout):
			return stateCandidate
		}
	}
}

func (m *Manager) becomeCandidate() state {
	o := m.runElection()
	for o != electionOutcomeInconclusive {
		o = m.runElection()
	}
	switch o {
	case electionOutcomeLeader:
		return stateLeader
	case electionOutcomeFollower:
		return stateFollower
	}
	return stateExit
}

func (m *Manager) runElection() electionOutcome {
	m.term++
	vr := pb.VoteRequest{
		CandidateID: m.id,
		Term:        m.term,
	}
	// Write vote request to each peer as a non blocking operation.
	// This is essential because, peers retry sending a protocol
	// requests indefinitly. Peers channel is non buffered.
	// We don't want to block an election due an inflight request to an
	// unavailable peer at the time.
	for _, p := range m.requestPeers {
		select {
		case p <- &vr:
		default:
			continue
		}
	}

	votes := 1
	// TODO: This must persistent
	voted := false
	for {
		select {
		case v := <-m.peerResponses:
			switch msg := v.(type) {
			case *pb.VoteResponse:
				if msg.Term == m.term {
					votes++
				}
				if votes >= ((len(m.requestPeers)+1)/2)+1 {
					return electionOutcomeLeader
				}
			}
		case v := <-m.request:
			switch msg := v.msg.(type) {
			case *pb.HeartbeatRequest:
				if msg.Term >= m.term {
					m.term = msg.Term
					return electionOutcomeFollower
				} else {
					res := pb.HeartbeatResponse{
						Term: m.term,
					}
					v.responder.respond(&res)
				}
			case *pb.VoteRequest:
				vote := !voted && msg.Term >= m.term
				res := pb.VoteResponse{
					Term: m.term,
					Yes:  vote,
				}
				voted = voted || vote
				v.responder.respond(&res)
			}
		case <-time.After(m.electionTimeout):
			return electionOutcomeInconclusive
		}
	}
}

func (m *Manager) becomeLeader() {

}

func NewManager(electionTimeout time.Duration, logger *zap.SugaredLogger) *Manager {
	return &Manager{
		electionTimeout: electionTimeout,
		logger:          logger,
	}
}

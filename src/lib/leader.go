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

type Leader struct {
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

func (l *Leader) Start() {
	n := l.becomeFollower()
	for n != stateExit {
		switch n {
		case stateCandidate:
			l.becomeCandidate()
		case stateLeader:
			l.becomeLeader()
		case stateFollower:
			l.becomeFollower()
		}
	}
}

func (l *Leader) becomeFollower() state {
	voted := false
	for {
		select {
		case v := <-l.request:
			switch msg := v.msg.(type) {
			case *pb.HeartbeatRequest:
				if msg.Term >= l.term {
					l.term = msg.Term
				} else {
					res := pb.HeartbeatResponse{
						Term: l.term,
					}
					v.responder.respond(&res)
				}
			case *pb.VoteRequest:
				vote := !voted && msg.Term >= l.term
				res := pb.VoteResponse{
					Term: l.term,
					Yes:  vote,
				}
				voted = voted || vote
				v.responder.respond(&res)

			}
		case <-time.After(l.electionTimeout):
			return stateCandidate
		}
	}
}

func (l *Leader) becomeCandidate() state {
	o := electionOutcomeInconclusive
	for o == electionOutcomeInconclusive {
		o = l.runElection()
	}
	switch o {
	case electionOutcomeLeader:
		return stateLeader
	case electionOutcomeFollower:
		return stateFollower
	}
	return stateExit
}

func (l *Leader) runElection() electionOutcome {
	l.term++
	vr := pb.VoteRequest{
		CandidateID: l.id,
		Term:        l.term,
	}
	// Write vote request to each peer as a non blocking operation.
	// This is essential because, peers retry sending a protocol
	// requests indefinitly. Peers channel is non buffered.
	// We don't want to block an election due an inflight request to an
	// unavailable peer at the time.
	for _, p := range l.requestPeers {
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
		case v := <-l.peerResponses:
			switch msg := v.(type) {
			case *pb.VoteResponse:
				if msg.Term == l.term {
					votes++
				}
				if votes >= ((len(l.requestPeers)+1)/2)+1 {
					return electionOutcomeLeader
				}
			}
		case v := <-l.request:
			switch msg := v.msg.(type) {
			case *pb.HeartbeatRequest:
				if msg.Term >= l.term {
					l.term = msg.Term
					return electionOutcomeFollower
				} else {
					res := pb.HeartbeatResponse{
						Term: l.term,
					}
					v.responder.respond(&res)
				}
			case *pb.VoteRequest:
				vote := !voted && msg.Term >= l.term
				res := pb.VoteResponse{
					Term: l.term,
					Yes:  vote,
				}
				voted = voted || vote
				v.responder.respond(&res)
			}
		case <-time.After(l.electionTimeout):
			return electionOutcomeInconclusive
		}
	}
}

func (l *Leader) becomeLeader() {
}

func NewLeader(electionTimeout time.Duration, logger *zap.SugaredLogger) *Leader {
	return &Leader{
		electionTimeout: electionTimeout,
		logger:          logger,
	}
}

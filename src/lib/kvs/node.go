package kvs

import (
	"slices"
	"time"

	"github.com/buddhike/libvegas/lib/kvs/pb"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type nodeState int

type request struct {
	msg      proto.Message
	response chan response
}

type response struct {
	peerID string
	msg    proto.Message
}

type log interface {
	append(*pb.Entry)
	get(int64) *pb.Entry
	purge(int64)
	len() int64
	last() *pb.Entry
}

type state interface {
	apply(*pb.Entry)
}

const (
	stateFollower nodeState = iota
	stateCandidate
	stateLeader
	stateExit
)

type Node struct {
	heartbeatTimeout time.Duration
	electionTimeout  time.Duration
	id               string
	logger           *zap.SugaredLogger
	currentLeader    string
	commitIndex      int64
	lastApplied      int64
	// Requests to this leader
	request chan request
	// Channels to send requests to peers
	peers            []chan<- request
	term             int64
	replicationState map[string]int64
	log              log
	state            state
	// Closed by user to notify that node must stop current activity and return
	stop chan struct{}
	// Closed by node to indicate the successful stop
	done chan struct{}
}

func (n *Node) Start() {
	s := stateFollower
forever:
	for s != stateExit {
		switch s {
		case stateFollower:
			n.becomeFollower()
		case stateCandidate:
			n.becomeCandidate()
		case stateLeader:
			n.becomeLeader()
		case stateExit:
			break forever
		}
	}
	close(n.done)
}

func (n *Node) becomeFollower() nodeState {
	timer := time.NewTimer(n.electionTimeout)
	voted := false
	for {
		select {
		case v := <-n.request:
			switch msg := v.msg.(type) {
			case *pb.AppendEntriesRequest:
				var rmsg *pb.AppendEntriesResponse
				if msg.Term < n.term {
					rmsg = &pb.AppendEntriesResponse{
						Term:    n.term,
						Success: false,
					}
				} else {
					n.currentLeader = msg.LeaderID
					n.term = msg.Term
					for _, e := range msg.Entries {
						n.log.append(e)
					}
					n.commit(msg.LeaderCommit)
					rmsg = &pb.AppendEntriesResponse{
						Term:    n.term,
						Success: true,
					}
					timer.Reset(n.electionTimeout)
				}
				res := response{
					peerID: n.id,
					msg:    rmsg,
				}
				v.response <- res
			case *pb.VoteRequest:
				vote := n.shouldVote(voted, msg.Term, msg.LastLogTerm, msg.LastLogIndex)
				voted = voted || vote
				if vote {
					timer.Reset(n.electionTimeout)
				}
				rmsg := pb.VoteResponse{
					Term:    n.term,
					Granted: vote,
				}
				res := response{
					peerID: n.id,
					msg:    &rmsg,
				}
				v.response <- res
			}
		case <-timer.C:
			return stateCandidate
		case <-n.stop:
			return stateExit
		}
	}
}

func (n *Node) commit(index int64) {
	n.commitIndex = index
	for i := n.commitIndex; n.lastApplied < n.commitIndex; n.lastApplied++ {
		entry := n.log.get(i)
		n.state.apply(entry)
	}
}

func (n *Node) shouldVote(alereadyVoted bool, peerCurrentTerm, peerLastEntryTerm, peerLastEntryIndex int64) bool {
	myLastEntry := n.log.last()
	isPeerLogAsUpToDate := (peerLastEntryTerm > myLastEntry.Term) || (myLastEntry.Term == peerLastEntryTerm && peerLastEntryIndex >= myLastEntry.Index)
	return !alereadyVoted && peerCurrentTerm >= n.term && isPeerLogAsUpToDate
}

func (n *Node) followerAppendEntries(msg *pb.AppendEntriesRequest) *pb.AppendEntriesResponse {
	if msg.Term < n.term {
		return &pb.AppendEntriesResponse{
			Term:    n.term,
			Success: false,
		}
	}
	n.currentLeader = msg.LeaderID
	n.term = msg.Term
	for _, e := range msg.Entries {
		n.log.append(e)
	}
	return &pb.AppendEntriesResponse{
		Term:    n.term,
		Success: true,
	}
}

func (n *Node) becomeCandidate() nodeState {
	o := stateCandidate
	for o == stateCandidate {
		o = n.runElection()
	}
	return 0
}

func (n *Node) runElection() nodeState {
	n.term++
	peerResponses := make(chan response)
	vr := pb.VoteRequest{
		CandidateID: n.id,
		Term:        n.term,
	}
	// Write vote request to each peer as a non blocking operation.
	// This is essential because, peers retry sending a protocol
	// requests indefinitly. Peers channel is non buffered.
	// We don't want to block an election due an inflight request to an
	// unavailable peer at the time.
	for _, p := range n.peers {
		select {
		case p <- request{
			msg:      &vr,
			response: peerResponses,
		}:
		default:
			continue
		}
	}

	votes := 1
	// TODO: This must persistent
	voted := false
	for {
		select {
		case v := <-peerResponses:
			switch msg := v.msg.(type) {
			case *pb.VoteResponse:
				if msg.Term == n.term {
					votes++
				}
				if votes >= ((len(n.peers)+1)/2)+1 {
					return stateLeader
				}
			}
		case v := <-n.request:
			switch msg := v.msg.(type) {
			case *pb.AppendEntriesRequest:
				if msg.Term >= n.term {
					n.term = msg.Term
					n.currentLeader = msg.LeaderID
					return stateFollower
				} else {
					v.response <- response{
						peerID: n.id,
						msg: &pb.AppendEntriesResponse{
							Term: n.term,
						},
					}
				}
			case *pb.VoteRequest:
				vote := n.shouldVote(voted, msg.Term, msg.LastLogTerm, msg.LastLogIndex)
				rmsg := pb.VoteResponse{
					Term:    n.term,
					Granted: vote,
				}
				res := response{
					peerID: n.id,
					msg:    &rmsg,
				}
				voted = voted || vote
				v.response <- res
			}
		case <-time.After(n.electionTimeout):
			return stateCandidate
		case <-n.stop:
			return stateExit
		}
	}
}

func (n *Node) becomeLeader() nodeState {
	for {
		select {
		case <-time.After(n.heartbeatTimeout):
			s := n.beatOnce()
			if s != stateLeader {
				return s
			}
		case <-n.request:
			// TODO: handle checkpoints
		case <-n.stop:
			return stateExit
		}
	}
}

func (n *Node) beatOnce() nodeState {
	responses := make(chan response)
	msg := pb.AppendEntriesRequest{
		Term:     n.term,
		LeaderID: n.id,
	}
	req := request{
		msg:      &msg,
		response: responses,
	}
	peers := slices.Clone(n.peers)
	nextPeerIdx := 0
	responseCount := 0
	for {
		select {
		case peers[nextPeerIdx] <- req:
			peers = peers[1:]
		case res := <-responses:
			msg := res.msg.(*pb.AppendEntriesResponse)
			if msg.Term == n.term {
				responseCount++
				if responseCount >= len(n.peers)/2 {
					return stateLeader
				}
			} else if msg.Term > n.term {
				n.term = msg.Term
				return stateFollower
			}
		default:
			nextPeerIdx++
			if nextPeerIdx >= len(peers) {
				nextPeerIdx = 0
			}
		}
	}
}

func NewNode(heartbeatTimeout, electionTimeout time.Duration, logger *zap.SugaredLogger) *Node {
	return &Node{
		heartbeatTimeout: heartbeatTimeout,
		electionTimeout:  electionTimeout,
		logger:           logger,
	}
}

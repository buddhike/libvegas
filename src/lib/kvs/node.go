package kvs

import (
	"maps"
	"math"
	"slices"
	"time"

	"github.com/buddhike/pebble/lib/kvs/pb"
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
	req    *request
}

type log interface {
	append(*pb.Entry)
	get(int64) *pb.Entry
	purge(int64)
	len() int64
	last() *pb.Entry
}

type State interface {
	Apply(*pb.Entry)
}

type Peer interface {
	ID() string
	Input() chan<- request
}

const (
	stateFollower nodeState = iota
	stateCandidate
	stateLeader
	stateExit
)

type Node struct {
	id               string
	heartbeatTimeout time.Duration
	electionTimeout  time.Duration
	logger           *zap.SugaredLogger
	currentLeader    string
	commitIndex      int64
	lastApplied      int64
	// Requests to this node
	request  chan request
	peers    []Peer
	term     int64
	log      log
	state    State
	votedFor string
	// Holds the request that transitioned this node to a follower.
	// becomeFollower handles the request prior to handling new messages
	// as a follower.
	pendingRequest *request
	// Closed by user to notify that node must stop current activity and return
	stop chan struct{}
	// Closed by node to indicate the successful stop
	done chan struct{}
}

func (n *Node) Start() {
	s := stateFollower
	for s != stateExit {
		switch s {
		case stateFollower:
			s = n.becomeFollower()
		case stateCandidate:
			s = n.becomeCandidate()
		case stateLeader:
			s = n.becomeLeader()
		}
	}
	close(n.done)
}

func (n *Node) becomeFollower() nodeState {
	// If node is becoming a follower because of a new term observered
	// from a peer, handle that request first.
	n.logger.Infof("pebble become follower %s", n.id)
	if n.pendingRequest != nil {
		switch n.pendingRequest.msg.(type) {
		case *pb.AppendEntriesRequest:
			n.appendEntries(n.pendingRequest)
		case *pb.VoteRequest:
			n.vote(*n.pendingRequest)
		}
	}
	n.pendingRequest = nil
	timer := time.NewTimer(n.electionTimeout)
	for {
		select {
		case req := <-n.request:
			switch msg := req.msg.(type) {
			case *pb.AppendEntriesRequest:
				if msg.Term < n.term {
					req.response <- response{
						peerID: n.id,
						msg: &pb.AppendEntriesResponse{
							Term:    n.term,
							Success: false,
						},
						req: &req,
					}
				} else {
					timer.Reset(n.electionTimeout)
					if n.term != msg.Term {
						n.currentLeader = msg.LeaderID
						n.updateNodeState(msg.Term, "")
					}
					n.appendEntries(&req)
				}
			case *pb.VoteRequest:
				if msg.Term < n.term {
					req.response <- response{
						peerID: n.id,
						msg: &pb.VoteResponse{
							Term:    n.term,
							Granted: false,
						},
						req: &req,
					}
				} else {
					timer.Reset(n.electionTimeout)
					if n.term != msg.Term {
						n.updateNodeState(msg.Term, "")
					}
					n.vote(req)
				}
			case *pb.ProposeRequest:
				// Followers only respond to peers. Proposals
				// from clients are rejected. Rejection response
				// indicate the leader id so that client and re-attempt
				// that request with the leader.
				req.response <- response{
					peerID: n.id,
					msg: &pb.PropseResponse{
						Accepted:      false,
						CurrentLeader: n.currentLeader,
					},
					req: &req,
				}
			}
		case <-timer.C:
			return stateCandidate
		case <-n.stop:
			return stateExit
		}
	}
}

func (n *Node) appendEntries(req *request) {
	msg := req.msg.(*pb.AppendEntriesRequest)
	var rmsg *pb.AppendEntriesResponse
	n.currentLeader = msg.LeaderID
	success := false
	if len(msg.Entries) > 0 {
		// We have entries to append
		// We must:
		// - ensure log matching property
		// - truncate the log if required
		if msg.PrevLogIndex == 0 {
			// Leader is saying that entries should be appended to the begining
			// of the log. Truncate the log to the begining and append entries.
			n.log.purge(0)
			for _, e := range msg.Entries {
				n.log.append(e)
			}
			success = true
		} else if msg.PrevLogIndex <= n.log.len() {
			// Leader is saying that entries should be appended after PrevLogIndex.
			// We also know that PrevLogIndex is valid in follower's log.
			// To ensure log matching property, we read the entry in that location
			// in follower and ensure the terms match.
			p := n.log.get(msg.PrevLogIndex)
			if p.Term == msg.PrevLogTerm {
				// Now that the terms are matching, truncate any entries past
				// PrevLogIndex and append entries.
				if msg.PrevLogIndex != n.log.len() {
					n.log.purge(msg.PrevLogIndex)
				}
				for _, e := range msg.Entries {
					n.log.append(e)
				}
				success = true
			}
		}
	} else {
		// This is a heartbeat request without any entries.
		// Follower simply acks it.
		success = true
	}
	// After any available entries are appended, ensure that
	// commit index is updated to match leader's commit.
	// If commit index is past the entry last applied, apply
	// those entries to state.
	if msg.LeaderCommit > 0 && n.log.len() > 0 && n.log.len() >= msg.LeaderCommit && n.log.get(msg.LeaderCommit).Term == n.term {
		n.commitIndex = msg.LeaderCommit
		for i := n.commitIndex; n.lastApplied < n.commitIndex; n.lastApplied++ {
			entry := n.log.get(i)
			n.state.Apply(entry)
		}
	}
	rmsg = &pb.AppendEntriesResponse{
		Term:    n.term,
		Success: success,
	}
	res := response{
		peerID: n.id,
		msg:    rmsg,
		req:    req,
	}
	req.response <- res
}

func (n *Node) vote(req request) {
	msg := req.msg.(*pb.VoteRequest)
	// Get the last entry from this node's log
	le := n.log.last()
	isPeerLogAsUpToDate := (le == nil) || (msg.LastLogTerm > le.Term) || (le.Term == msg.LastLogTerm && msg.LastLogIndex >= le.Index)
	alreadyVotedThisCandidateInSameTerm := msg.CandidateID == n.votedFor && n.term == msg.Term
	termIsCurrentOrNew := msg.Term >= n.term
	haventVotedYet := n.votedFor == ""
	granted := (haventVotedYet || alreadyVotedThisCandidateInSameTerm) && termIsCurrentOrNew && isPeerLogAsUpToDate
	if granted {
		n.updateNodeState(n.term, msg.CandidateID)
	}
	rmsg := pb.VoteResponse{
		Term:    n.term,
		Granted: granted,
	}
	res := response{
		peerID: n.id,
		msg:    &rmsg,
		req:    &req,
	}
	req.response <- res
}

func (n *Node) updateNodeState(term int64, votedFor string) {
	n.term = term
	n.votedFor = votedFor
}

func (n *Node) becomeCandidate() nodeState {
	n.logger.Infof("pebble became candidate: %s", n.id)
	result := stateCandidate
	for result == stateCandidate {
		result = n.runElection()
	}
	return result
}

func (n *Node) runElection() nodeState {
	n.term++
	n.updateNodeState(n.term, "")
	peerResponses := make(chan response)
	numOutstandingResponses := 0
	defer func() {
		go n.drain(peerResponses, numOutstandingResponses)
	}()
	vr := pb.VoteRequest{
		CandidateID: n.id,
		Term:        n.term,
	}
	peers := slices.Clone(n.peers)
	votes := 1
	req := request{
		msg:      &vr,
		response: peerResponses,
	}
	nextPeer := peers[0]
	nextPeerInput := nextPeer.Input()
	quorumSize := len(n.peers) + 1
	timer := time.NewTimer(n.electionTimeout)
	for {
		select {
		case nextPeerInput <- req:
			timer.Reset(n.electionTimeout)
			numOutstandingResponses++
			peers = peers[1:]
			if len(peers) > 0 {
				nextPeer = peers[0]
				nextPeerInput = nextPeer.Input()
			} else {
				nextPeer = nil
				nextPeerInput = nil
			}
		case v := <-peerResponses:
			numOutstandingResponses--
			timer.Reset(n.electionTimeout)
			switch msg := v.msg.(type) {
			case *pb.VoteResponse:
				if msg.Term == n.term && msg.Granted {
					votes++
				}
				if votes >= (quorumSize/2)+1 {
					return stateLeader
				}
			}
		case v := <-n.request:
			switch msg := v.msg.(type) {
			case *pb.AppendEntriesRequest:
				if msg.Term >= n.term {
					if msg.Term > n.term {
						n.updateNodeState(msg.Term, "")
					}
					n.pendingRequest = &v
					return stateFollower
				} else {
					v.response <- response{
						peerID: n.id,
						msg: &pb.AppendEntriesResponse{
							Term:    n.term,
							Success: false,
						},
					}
				}
			case *pb.VoteRequest:
				if n.term > msg.Term {
					v.response <- response{
						peerID: n.id,
						msg: &pb.VoteResponse{
							Term:    n.term,
							Granted: false,
						},
					}
				} else if n.term == msg.Term {
					timer.Reset(n.electionTimeout)
					n.vote(req)
				} else {
					n.pendingRequest = &v
					return stateFollower
				}
			case *pb.ProposeRequest:
				// Candidates only respond to peers. Proposals
				// from clients are rejected.
				v.response <- response{
					peerID: n.id,
					msg: &pb.PropseResponse{
						Accepted:      false,
						CurrentLeader: "",
					},
				}
			}
		case <-timer.C:
			return stateCandidate
		case <-n.stop:
			return stateExit
		default:
			if len(peers) > 1 {
				peers = peers[1:]
				peers = append(peers, nextPeer)
				nextPeer = peers[0]
				nextPeerInput = nextPeer.Input()
			}
		}
	}
}

func (n *Node) becomeLeader() nodeState {
	n.logger.Infof("pebble became leader: %s", n.id)
	nextIdx := make(map[string]int64)
	sendHeartbeat := make(map[string]bool)
	matchIdx := make(map[string]int64)
	timer := time.NewTimer(time.Duration(0))
	peerResponses := make(chan response)
	pendingProposals := make(map[int64]request)
	for _, p := range n.peers {
		nextIdx[p.ID()] = n.log.len()
		sendHeartbeat[p.ID()] = true
		matchIdx[p.ID()] = 0
	}

	nextPeerIdx := 0
	for {
		nextPeer := n.peers[nextPeerIdx]
		nextPeerInput := nextPeer.Input()
		var appendEntriesReq request
		if sendHeartbeat[nextPeer.ID()] {
			m := &pb.AppendEntriesRequest{
				Term:         n.term,
				LeaderID:     n.id,
				LeaderCommit: n.commitIndex,
			}
			appendEntriesReq = request{
				msg:      m,
				response: peerResponses,
			}
		} else if n.log.len() >= nextIdx[nextPeer.ID()] {
			entries := make([]*pb.Entry, (n.log.len()-nextIdx[nextPeer.ID()])+1)
			for i := range len(entries) {
				entries[i] = n.log.get(n.log.len() + int64(i))
			}

			m := &pb.AppendEntriesRequest{
				Term:         n.term,
				LeaderID:     n.id,
				LeaderCommit: n.commitIndex,
				Entries:      entries,
			}
			appendEntriesReq = request{
				msg:      m,
				response: peerResponses,
			}
		} else {
			nextPeer = nil
		}

		if nextPeer == nil {
			timer.Reset(n.heartbeatTimeout)
		} else {
			timer.Stop()
		}

		select {
		case nextPeerInput <- appendEntriesReq:
			nextPeerIdx++
			if nextPeerIdx >= len(n.peers) {
				nextPeerIdx = 0
			}
		case <-timer.C:
			for k := range maps.Keys(sendHeartbeat) {
				sendHeartbeat[k] = true
			}
		case res := <-peerResponses:
			msg := res.msg.(*pb.AppendEntriesResponse)
			if msg.Term > n.term {
				n.term = msg.Term
				return stateFollower
			}
			reqMsg := res.req.msg.(*pb.AppendEntriesRequest)
			if msg.Success && len(reqMsg.Entries) > 0 {
				matchIdx[res.peerID] = reqMsg.Entries[len(reqMsg.Entries)-1].Index
				nextIdx[res.peerID] = reqMsg.Entries[len(reqMsg.Entries)-1].Index + 1
			} else {
				nextIdx[res.peerID] = nextIdx[res.peerID] - 1
			}
			smallestMatchIndex := int64(math.MaxInt64)
			for k := range maps.Keys(matchIdx) {
				if matchIdx[k] < int64(smallestMatchIndex) {
					smallestMatchIndex = matchIdx[k]
				}
			}
			if n.commitIndex < smallestMatchIndex {
				e := n.log.get(smallestMatchIndex)
				if e.Term == n.term {
					n.commitIndex = smallestMatchIndex
					for i := n.commitIndex; n.lastApplied < n.commitIndex; n.lastApplied++ {
						entry := n.log.get(i)
						n.state.Apply(entry)
					}

					for k := range maps.Keys(pendingProposals) {
						if k <= n.commitIndex {
							r := pendingProposals[k]
							pendingProposals[k].response <- response{
								peerID: n.id,
								req:    &r,
								msg: &pb.PropseResponse{
									Accepted: true,
								},
							}
						}
					}
				}
			}
		case req := <-n.request:
			switch r := req.msg.(type) {
			case *pb.ProposeRequest:
				e := &pb.Entry{
					Index:     n.log.len() + 1,
					Term:      n.term,
					Operation: r.Operation,
					Key:       r.Key,
					Value:     r.Value,
				}
				n.log.append(e)
				pendingProposals[e.Index] = req
			case *pb.AppendEntriesRequest:
				if r.Term > n.term {
					// TODO: Cancel pending proposals
					n.updateNodeState(r.Term, "")
					n.pendingRequest = &req
					return stateFollower
				}
				res := response{
					peerID: n.id,
					msg: &pb.AppendEntriesResponse{
						Term:    n.term,
						Success: false,
					},
					req: &req,
				}
				req.response <- res
			case *pb.VoteRequest:
				if r.Term > n.term {
					n.updateNodeState(r.Term, "")
					n.pendingRequest = &req
					return stateFollower
				}
				res := response{
					peerID: n.id,
					msg: &pb.VoteResponse{
						Term:    n.term,
						Granted: false,
					},
					req: &req,
				}
				req.response <- res
			}
		case <-n.stop:
			return stateExit
		default:
			nextPeerIdx++
			if nextPeerIdx >= len(n.peers) {
				nextPeerIdx = 0
			}
		}
	}
}

func (n *Node) drain(c chan response, numberOfOutstandingResponses int) {
	for numberOfOutstandingResponses != 0 {
		<-c
		numberOfOutstandingResponses--
	}
}

func (n *Node) ID() string {
	return n.id
}

func (n *Node) Input() chan<- request {
	return n.request
}

func (n *Node) Done() <-chan struct{} {
	return n.done
}

func (n *Node) SetPeers(peers []Peer) {
	n.peers = peers
}

func NewNode(id string, heartbeatTimeout, electionTimeout time.Duration, log log, state State, stop chan struct{}, logger *zap.SugaredLogger) *Node {
	return &Node{
		id:               id,
		heartbeatTimeout: heartbeatTimeout,
		electionTimeout:  electionTimeout,
		log:              log,
		state:            state,
		stop:             stop,
		done:             make(chan struct{}),
		request:          make(chan request),
		logger:           logger,
	}
}

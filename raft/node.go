package raft

import (
	"context"
	"log"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/Sjk4824/distributed_kv_store/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Role int

const (
	Follower  Role = iota //passive role, just waits for the hearbeats
	Candidate             //asks for votes to become the leader
	Leader                //the actual leader who sends heartbeats to prove its alive, handles reqs from te=he clients and primary writer node.
)

func (r Role) String() string {
	switch r {
	case Follower:
		return "follower"
	case Candidate:
		return "candidate"
	case Leader:
		return "leader"
	default:
		return "unknown"
	}
}

type Node struct {
	api.UnimplementedRaftServer
	mu       sync.Mutex
	id       string
	addr     string
	peers    []string
	conns    map[string]*grpc.ClientConn
	clients  map[string]api.RaftClient
	role     Role
	term     uint64
	votedFor string
	leaderID string
	log      []*api.LogEntry

	commitIndex uint64
	lastApplied uint64

	nextIndex  map[string]uint64
	matchIndex map[string]uint64

	resetElectionCh chan struct{}
	stopCh          chan struct{}
	doneCh          chan struct{}

	electionMin time.Duration
	electionMax time.Duration
	heartbeat   time.Duration
	rng         *rand.Rand
}

func NewNode(id, addr string, peers []string) *Node {
	return &Node{
		id:              id,
		addr:            addr,
		peers:           peers,
		conns:           make(map[string]*grpc.ClientConn),
		clients:         make(map[string]api.RaftClient),
		role:            Follower,
		resetElectionCh: make(chan struct{}, 1),
		stopCh:          make(chan struct{}),
		doneCh:          make(chan struct{}),
		log:             make([]*api.LogEntry, 0),
		nextIndex:       make(map[string]uint64),
		matchIndex:      make(map[string]uint64),

		electionMin: 1500 * time.Millisecond,
		electionMax: 3000 * time.Millisecond,
		heartbeat:   100 * time.Millisecond,

		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (n *Node) Start() {
	go n.run()
}

func (n *Node) Stop() {
	close(n.stopCh)
	<-n.doneCh
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, conn := range n.conns {
		conn.Close()
	}
}

func (n *Node) RoleTermLeader() (role Role, term uint64, leader string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.role, n.term, n.leaderID
}

func (n *Node) IsLeader() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.role == Leader
}

// doubt about what this clients and peers are? ?
func (n *Node) getClient(peer string) (api.RaftClient, error) {
	n.mu.Lock()
	if c, ok := n.clients[peer]; ok {
		n.mu.Unlock()
		return c, nil
	}
	n.mu.Unlock()

	conn, err := grpc.Dial(peer, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	client := api.NewRaftClient(conn)

	n.mu.Lock()
	if existing, ok := n.clients[peer]; ok {
		n.mu.Unlock()
		_ = conn.Close()
		return existing, nil
	}
	n.conns[peer] = conn
	n.clients[peer] = client
	n.mu.Unlock()

	return client, nil
}

func (n *Node) run() {
	defer close(n.doneCh)

	for {
		select {
		case <-n.stopCh:
			return
		default:
		}

		n.mu.Lock()
		role := n.role
		n.mu.Unlock()

		switch role {
		case Leader:
			n.leaderLoop()
		default:
			n.followerCandidateLoop()
		}
	}
}

func (n *Node) followerCandidateLoop() {
	timeout := n.randElectionTimeout()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-n.stopCh:
			return
		case <-n.resetElectionCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(n.randElectionTimeout())
		case <-timer.C: //if the timer expires, it means we haven't received a heartbeat in time, so we start an election to try to become the leader.
			n.startElection()
			return
		}

		n.mu.Lock()
		if n.role == Leader {
			n.mu.Unlock()
			return
		}
		n.mu.Unlock()
	}
}

func (n *Node) leaderLoop() {
	t := time.NewTicker(n.heartbeat)
	defer t.Stop()

	n.sendHeartbeats()
	for {
		select {
		case <-n.stopCh:
			return
		case <-t.C:
			n.sendHeartbeats()
		}

		n.mu.Lock()
		if n.role != Leader {
			n.mu.Unlock()
			return
		}
		n.mu.Unlock()
	}
}

func (n *Node) randElectionTimeout() time.Duration {
	// random in [min, max)
	delta := n.electionMax - n.electionMin
	if delta <= 0 {
		return n.electionMin
	}
	return n.electionMin + time.Duration(n.rng.Int63n(int64(delta)))
}

func (n *Node) startElection() {
	n.mu.Lock()
	// become candidate
	n.role = Candidate
	n.term++
	term := n.term
	log.Printf("[raft %s] became CANDIDATE term=%d", n.id, n.term)
	n.votedFor = n.id
	n.leaderID = ""

	lastIdx, lastTerm := n.lastLogIndexTermLocked()

	n.mu.Unlock()

	votes := 1 // self vote
	needed := (len(n.peers)+1)/2 + 1

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	voteCh := make(chan bool, len(n.peers))

	for _, p := range n.peers {
		peer := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := n.getClient(peer)
			if err != nil {
				voteCh <- false
				return
			}
			resp, err := c.RequestVote(ctx, &api.RequestVoteRequest{
				CandidateId:  n.id,
				Term:         term,
				LastLogIndex: lastIdx,
				LastLogTerm:  lastTerm,
			})
			if err != nil {
				voteCh <- false
				return
			}

			// If peer has higher term, step down
			n.mu.Lock()
			if resp.GetTerm() > n.term {
				n.term = resp.GetTerm()
				n.role = Follower
				log.Printf("[raft %s] stepping down to FOLLOWER: higher term %d seen (was %d)", n.id, resp.GetTerm(), n.term)
				n.votedFor = ""
				n.leaderID = ""
				n.mu.Unlock()
				voteCh <- false
				return
			}
			n.mu.Unlock()

			voteCh <- resp.GetVoteGranted()
		}()
	}

	wg.Wait()
	close(voteCh)

	for granted := range voteCh {
		if granted {
			votes++
		}
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	// Only become leader if we're still candidate in same term
	if n.role == Candidate && n.term == term && votes >= needed {
		n.role = Leader
		log.Printf("[raft %s] became LEADER term=%d (votes=%d/%d)", n.id, term, votes, len(n.peers)+1)
		n.leaderID = n.id
		// n.votedFor can remain set
		// immediately notify follower loop timers by "reset" (optional)
	}
}

func (n *Node) sendHeartbeats() {
	n.mu.Lock()
	if n.role != Leader {
		n.mu.Unlock()
		return
	}
	term := n.term
	leaderID := n.id
	leaderCommit := n.commitIndex

	// Initialize nextIndex/matchIndex on first leader election
	if len(n.nextIndex) == 0 {
		lastIdx, _ := n.lastLogIndexTermLocked()
		for _, p := range n.peers {
			n.nextIndex[p] = lastIdx + 1
			n.matchIndex[p] = 0
		}
	}
	n.mu.Unlock()

	// Send AppendEntries to each peer in parallel
	var wg sync.WaitGroup
	for _, p := range n.peers {
		peer := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			n.replicateToPeer(peer, term, leaderID, leaderCommit)
		}()
	}
	wg.Wait()
}

func (n *Node) lastLogIndexTermLocked() (uint64, uint64) {
	if len(n.log) == 0 {
		return 0, 0
	}
	last := n.log[len(n.log)-1]
	return last.GetIndex(), last.GetTerm()
}

func (n *Node) AppendLog(cmd *api.Command) uint64 {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.role != Leader {
		return 0
	}

	index := uint64(len(n.log)) + 1
	entry := &api.LogEntry{
		Term:  n.term,
		Index: index,
		Cmd:   cmd,
	}
	n.log = append(n.log, entry)
	return index
}

func (n *Node) replicateToPeer(peer string, term uint64, leaderID string, leaderCommit uint64) {
	n.mu.Lock()
	nextIdx := n.nextIndex[peer]

	var prevLogIdx, prevLogTerm uint64
	if nextIdx > 1 {
		prevLogIdx = nextIdx - 1
		prevLogTerm = n.log[prevLogIdx-1].GetTerm()
	}

	var entriesToSend []*api.LogEntry
	if nextIdx-1 < uint64(len(n.log)) {
		entriesToSend = n.log[nextIdx-1:]
	}
	n.mu.Unlock()

	c, err := n.getClient(peer)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	resp, err := c.AppendEntries(ctx, &api.AppendEntriesRequest{
		LeaderId:     leaderID,
		Term:         term,
		PrevLogIndex: prevLogIdx,
		PrevLongTerm: prevLogTerm,
		Entries:      entriesToSend,
		LeaderCommit: leaderCommit,
	})
	if err != nil {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if resp.GetTerm() > n.term {
		n.term = resp.GetTerm()
		n.role = Follower
		n.votedFor = ""
		n.leaderID = ""
		n.nextIndex = make(map[string]uint64)
		n.matchIndex = make(map[string]uint64)
		return
	}

	// Success: advance nextIndex and matchIndex
	if resp.GetSuccess() {
		n.matchIndex[peer] = resp.GetMatchIndec() // use the confirmed index from response
		n.nextIndex[peer] = resp.GetMatchIndec() + 1
		n.updateCommitIndex()
	} else {
		// Failure: decrement nextIndex and retry
		if n.nextIndex[peer] > 1 {
			n.nextIndex[peer]--
		}
	}

}

func (n *Node) updateCommitIndex() {
	// Find the highest index that is replicated on a majority
	if len(n.matchIndex) == 0 {
		return
	}

	// Collect all matchIndex values
	indices := make([]uint64, 0, len(n.matchIndex))
	for _, idx := range n.matchIndex {
		indices = append(indices, idx)
	}
	// Sort to find median
	sort.Slice(indices, func(i, j int) bool { return indices[i] < indices[j] })

	// Majority index is at position len/2 (e.g., 3 peers -> index at pos 1)
	majorityIdx := indices[len(indices)/2]

	// Only advance commitIndex if the entry is from current term
	if majorityIdx > n.commitIndex && majorityIdx-1 < uint64(len(n.log)) {
		if n.log[majorityIdx-1].GetTerm() == n.term {
			n.commitIndex = majorityIdx
			log.Printf("[raft %s] advanced commitIndex to %d", n.id, n.commitIndex)
		}
	}
}

// ApplyEntries applies all entries up to commitIndex to a callback
// This is called by the KV server to apply committed commands
func (n *Node) ApplyEntries(apply func(*api.LogEntry) error) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	for n.lastApplied < n.commitIndex {
		n.lastApplied++
		idx := n.lastApplied - 1
		if idx >= uint64(len(n.log)) {
			break // shouldn't happen, but be safe
		}
		entry := n.log[idx]
		if entry == nil {
			continue
		}
		if err := apply(entry); err != nil {
			log.Printf("[raft %s] failed to apply entry %d: %v", n.id, entry.GetIndex(), err)
			return err
		}
	}
	return nil
}

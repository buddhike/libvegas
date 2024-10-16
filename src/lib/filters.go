package lib

import (
	"crypto/md5"
	"encoding/hex"
	"sync"

	"github.com/buddhike/pebble/lib/pb"
)

// invalidMappingFilter filters the records that are incorrectly written to a
// shard following a split or merge operation.
type invalidMappingFilter struct {
}

func (f *invalidMappingFilter) Apply(c *shardReaderContext, r *pb.Record) bool {
	return r.ShardID != c.shardID
}

func newInvalidMappingFilter() *invalidMappingFilter {
	return &invalidMappingFilter{}
}

type dedupFilter struct {
	l    *sync.Mutex
	seen map[string]bool
}

func (s *dedupFilter) Apply(c *shardReaderContext, r *pb.UserRecord) bool {
	s.l.Lock()
	defer s.l.Unlock()

	h := md5.Sum(r.RecordID)
	k := hex.EncodeToString(h[:])
	if _, ok := s.seen[k]; ok {
		return ok
	}
	s.seen[k] = true
	return false
}

func newDedupFilter() *dedupFilter {
	return &dedupFilter{
		l:    &sync.Mutex{},
		seen: make(map[string]bool),
	}
}

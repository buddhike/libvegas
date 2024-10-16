package kvs

import "github.com/buddhike/pebble/lib/kvs/pb"

type inMemoryLog struct {
	entries []*pb.Entry
}

func (l *inMemoryLog) append(entry *pb.Entry) {
	l.entries = append(l.entries, entry)
}

func (l *inMemoryLog) get(idx int64) *pb.Entry {
	return l.entries[idx]
}

func (l *inMemoryLog) purge(after int64) {
	l.entries = l.entries[after:0]
}

func (l *inMemoryLog) len() int64 {
	return int64(len(l.entries))
}

func (l *inMemoryLog) last() *pb.Entry {
	if len(l.entries) > 0 {
		return l.entries[len(l.entries)-1]
	}
	return nil
}

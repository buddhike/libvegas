package kvs

import "github.com/buddhike/libvegas/lib/kvs/pb"

type inMemoryMap struct {
	i map[string]string
}

func (m *inMemoryMap) Apply(entry *pb.Entry) {
	m.i[string(entry.Key)] = string(entry.Value)
}

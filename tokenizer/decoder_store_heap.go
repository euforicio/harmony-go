//go:build !goexperiment.arenas

package tokenizer

// Heap-backed token store using a single blob and offset table.
// This is the default implementation and serves as the fallback when
// arenas are not enabled.

type heapStore struct {
	arr [][]byte // direct references to token byte slices
}

func newTokenStore(pairs [][2]any) (tokenStore, error) {
	// Determine max id and collect per-id bytes
	maxID := uint32(0)
	for _, p := range pairs {
		id, _ := p[1].(uint32)
		if id > maxID {
			maxID = id
		}
	}
	size := int(maxID) + 1
	tmp := make([][]byte, size)
	for _, p := range pairs {
		b, _ := p[0].([]byte)
		id, _ := p[1].(uint32)
		if tmp[int(id)] == nil {
			tmp[int(id)] = b
		}
	}
	return &heapStore{arr: tmp}, nil
}

func (s *heapStore) AppendInto(dst *[]byte, id uint32) bool {
	if int(id) >= len(s.arr) {
		return false
	}
	b := s.arr[id]
	if b == nil {
		return false
	}
	*dst = append(*dst, b...)
	return true
}

func (s *heapStore) Close() {}

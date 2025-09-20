//go:build goexperiment.arenas

package tokenizer

import "arena"

// Arena-backed token store. All storage lives in a dedicated arena.
// AppendInto copies from the arena blob into the destination to avoid
// leaking arena-backed slices to the heap.
type arenaStore struct {
	a    *arena.Arena
	blob []byte
	off  []uint32
}

func newTokenStore(pairs [][2]any) (tokenStore, error) {
	a := arena.NewArena()
	// Determine max id and collect per-id lengths
	maxID := uint32(0)
	for _, p := range pairs {
		id, _ := p[1].(uint32)
		if id > maxID {
			maxID = id
		}
	}
	size := int(maxID) + 1
	lens := arena.MakeSlice[uint32](a, size, size)
	total := 0
	for _, p := range pairs {
		b, _ := p[0].([]byte)
		id, _ := p[1].(uint32)
		if lens[int(id)] == 0 {
			lens[int(id)] = uint32(len(b))
			total += len(b)
		}
	}
	blob := arena.MakeSlice[byte](a, total, total)
	off := arena.MakeSlice[uint32](a, size+1, size+1)
	pos := 0
	for i := 0; i < size; i++ {
		off[i] = uint32(pos)
		n := int(lens[i])
		if n > 0 {
			// find the bytes for id i (second pass)
			// Note: this is O(nIds + nPairs); still fine for one-time init.
			for _, p := range pairs {
				id, _ := p[1].(uint32)
				if int(id) != i {
					continue
				}
				b, _ := p[0].([]byte)
				copy(blob[pos:pos+n], b)
				break
			}
			pos += n
		}
	}
	off[size] = uint32(pos)
	return &arenaStore{a: a, blob: blob, off: off}, nil
}

func (s *arenaStore) AppendInto(dst *[]byte, id uint32) bool {
	if int(id) >= len(s.off)-1 {
		return false
	}
	a := s.off[id]
	b := s.off[id+1]
	if a == b {
		return false
	}
	*dst = append(*dst, s.blob[a:b]...)
	return true
}

func (s *arenaStore) Close() { s.a.Free() }

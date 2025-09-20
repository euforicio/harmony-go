package tokenizer

import (
	"errors"
	"sync"
)

// Rank represents the priority/rank of a token pair in BPE encoding.
type Rank = uint32

type coreBPE struct {
	enc        map[string]Rank // key: raw bytes as string
	dec        tokenStore
	specialEnc map[string]Rank
	specialDec map[Rank][]byte
	seg        Segmenter
	partsPool  sync.Pool
	tokenPool  sync.Pool
}

func newCoreBPE(encoderPairs [][2]any, specials map[string]Rank, seg Segmenter) (*coreBPE, error) {
	enc := make(map[string]Rank, len(encoderPairs))
	for _, p := range encoderPairs {
		b, _ := p[0].([]byte)
		r, _ := p[1].(Rank)
		enc[string(b)] = r
	}
	dec, err := newTokenStore(encoderPairs)
	if err != nil {
		return nil, err
	}
	specialEnc := make(map[string]Rank, len(specials))
	specialDec := make(map[Rank][]byte, len(specials))
	for k, v := range specials {
		specialEnc[k] = v
		specialDec[v] = []byte(k)
	}
	return &coreBPE{
		enc:        enc,
		dec:        dec,
		specialEnc: specialEnc,
		specialDec: specialDec,
		seg:        seg,
		partsPool:  sync.Pool{New: func() any { b := make([]part, 0, 64); return &b }},
		tokenPool:  sync.Pool{New: func() any { b := make([]uint32, 0, 32); return &b }},
	}, nil
}

func (b *coreBPE) DecodeBytes(tokens []uint32) ([]byte, error) {
	var out []byte
	if err := b.DecodeBytesInto(&out, tokens); err != nil {
		return nil, err
	}
	return out, nil
}

func (b *coreBPE) DecodeUTF8(tokens []uint32) (string, error) {
	bs, err := b.DecodeBytes(tokens)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

// DecodeBytesInto appends the decoded bytes for the provided tokens
// into dst, avoiding intermediate slice allocations.
func (b *coreBPE) DecodeBytesInto(dst *[]byte, tokens []uint32) error {
	buf := *dst
	for _, t := range tokens {
		if b.dec.AppendInto(&buf, t) {
			continue
		}
		if v, ok := b.specialDec[t]; ok {
			buf = append(buf, v...)
			continue
		}
		return errors.New("invalid token for decoding")
	}
	*dst = buf
	return nil
}

func (b *coreBPE) IsSpecialToken(id uint32) bool { _, ok := b.specialDec[id]; return ok }

func (b *coreBPE) EncodeWithSpecialTokens(text string) []uint32 {
	allowed := make(map[string]struct{}, len(b.specialEnc))
	for s := range b.specialEnc {
		allowed[s] = struct{}{}
	}
	toks, _ := b.Encode(text, allowed)
	return toks
}

// EncodeWithSpecialTokensInto appends tokens for text allowing all special
// tokens directly when present.
func (b *coreBPE) EncodeWithSpecialTokensInto(text string, out *[]uint32) int {
	allowed := make(map[string]struct{}, len(b.specialEnc))
	for s := range b.specialEnc {
		allowed[s] = struct{}{}
	}
	return b.encodeInto(text, allowed, out)
}

func (b *coreBPE) EncodeOrdinary(text string) []uint32 {
	toks, _ := b.Encode(text, nil)
	return toks
}

// EncodeIntoOrdinary appends tokens for text into out without creating
// an intermediate result slice.
func (b *coreBPE) EncodeIntoOrdinary(text string, out *[]uint32) int {
	return b.encodeInto(text, nil, out)
}

// Encode mirrors the upstream behaviour: scans text with segmenter, merges via BPE; allowedSpecial indicates which
// specials may be emitted directly.
func (b *coreBPE) Encode(text string, allowedSpecial map[string]struct{}) ([]uint32, int) {
	var out []uint32
	lastPieceLen := 0
	i := 0
	hasSpecials := len(allowedSpecial) > 0
	for i < len(text) {
		// Special token check at position i
		if hasSpecials {
			if tok, n := b.matchSpecialAt(text, i, allowedSpecial); n > 0 {
				out = append(out, tok)
				i += n
				lastPieceLen = 0
				continue
			}
		}
		// Next segment
		start := i
		end := b.seg.Next(text, i)
		if end <= start { // safety
			end = start + 1
		}
		piece := text[start:end]
		if id, ok := b.enc[piece]; ok {
			out = append(out, id)
			lastPieceLen = 1
		} else {
			toks, release := b.bytePairEncode(piece)
			out = append(out, toks...)
			lastPieceLen = len(toks)
			release()
		}
		i = end
	}
	return out, lastPieceLen
}

// encodeInto is the in-place variant of Encode.
func (b *coreBPE) encodeInto(text string, allowedSpecial map[string]struct{}, out *[]uint32) int {
	lastPieceLen := 0
	i := 0
	hasSpecials := len(allowedSpecial) > 0
	for i < len(text) {
		if hasSpecials {
			if tok, n := b.matchSpecialAt(text, i, allowedSpecial); n > 0 {
				*out = append(*out, tok)
				i += n
				lastPieceLen = 0
				continue
			}
		}
		start := i
		end := b.seg.Next(text, i)
		if end <= start {
			end = start + 1
		}
		piece := text[start:end]
		if id, ok := b.enc[piece]; ok {
			*out = append(*out, id)
			lastPieceLen = 1
		} else {
			toks, release := b.bytePairEncode(piece)
			*out = append(*out, toks...)
			lastPieceLen = len(toks)
			release()
		}
		i = end
	}
	return lastPieceLen
}

func (b *coreBPE) matchSpecialAt(s string, i int, allowed map[string]struct{}) (uint32, int) {
	// Linear probe: all Harmony specials are distinct and short; optimize later with trie if needed.
	// Longest first to ensure greedy match.
	// Note: only emit if present in allowed set.
	maxLen := 0
	var id uint32
	for lit, tok := range b.specialEnc {
		if _, ok := allowed[lit]; !ok {
			continue
		}
		if len(lit) > len(s)-i {
			continue
		}
		if s[i:i+len(lit)] == lit && len(lit) > maxLen {
			maxLen = len(lit)
			id = tok
		}
	}
	if maxLen == 0 {
		return 0, 0
	}
	return id, maxLen
}

// Byte pair encode identical to the upstream logic using ranks map.
func (b *coreBPE) bytePairEncode(piece string) ([]uint32, func()) {
	if len(piece) == 1 {
		buf, release := b.acquireTokens(1)
		buf = append(buf[:0], b.enc[piece])
		return buf, release
	}
	parts, releaseParts := b.bytePairMerge(piece)
	toks, releaseTokens := b.acquireTokens(len(parts))
	toks = toks[:0]
	for w := 0; w+1 < len(parts); w++ {
		toks = append(toks, b.enc[piece[parts[w].start:parts[w+1].start]])
	}
	release := func() {
		releaseParts()
		releaseTokens()
	}
	return toks, release
}

type part struct {
	start int
	rank  uint32
}

func (b *coreBPE) getRank(piece string, parts []part, i int) uint32 {
	if i+3 < len(parts) {
		if r, ok := b.enc[piece[parts[i].start:parts[i+3].start]]; ok {
			return r
		}
	}
	return ^uint32(0)
}

func (b *coreBPE) bytePairMerge(piece string) ([]part, func()) {
	parts, release := b.acquireParts(len(piece) + 2)
	parts = parts[:0]
	minRank := struct {
		rank uint32
		idx  int
	}{rank: ^uint32(0), idx: -1}
	for i := 0; i < len(piece)-1; i++ {
		r, ok := b.enc[piece[i:i+2]]
		if !ok {
			r = ^uint32(0)
		}
		if r < minRank.rank {
			minRank = struct {
				rank uint32
				idx  int
			}{r, i}
		}
		parts = append(parts, part{start: i, rank: r})
	}
	parts = append(parts, part{start: len(piece) - 1, rank: ^uint32(0)})
	parts = append(parts, part{start: len(piece), rank: ^uint32(0)})

	for minRank.rank != ^uint32(0) {
		i := minRank.idx
		if i > 0 {
			parts[i-1].rank = b.getRank(piece, parts, i-1)
		}
		parts[i].rank = b.getRank(piece, parts, i)
		parts = append(parts[:i+1], parts[i+2:]...)
		minRank = struct {
			rank uint32
			idx  int
		}{rank: ^uint32(0), idx: -1}
		for j := 0; j < len(parts)-1; j++ {
			if parts[j].rank < minRank.rank {
				minRank = struct {
					rank uint32
					idx  int
				}{parts[j].rank, j}
			}
		}
	}
	return parts, release
}

func (b *coreBPE) acquireParts(capHint int) ([]part, func()) {
	var p *[]part
	if v := b.partsPool.Get(); v != nil {
		p = v.(*[]part)
		if cap(*p) < capHint {
			buf := make([]part, 0, capHint)
			p = &buf
		} else {
			*p = (*p)[:0]
		}
	} else {
		buf := make([]part, 0, capHint)
		p = &buf
	}
	release := func() {
		if cap(*p) > 1<<12 {
			return
		}
		*p = (*p)[:0]
		b.partsPool.Put(p)
	}
	return *p, release
}

func (b *coreBPE) acquireTokens(capHint int) ([]uint32, func()) {
	var p *[]uint32
	if v := b.tokenPool.Get(); v != nil {
		p = v.(*[]uint32)
		if cap(*p) < capHint {
			buf := make([]uint32, 0, capHint)
			p = &buf
		} else {
			*p = (*p)[:0]
		}
	} else {
		buf := make([]uint32, 0, capHint)
		p = &buf
	}
	release := func() {
		if cap(*p) > 1<<12 {
			return
		}
		*p = (*p)[:0]
		b.tokenPool.Put(p)
	}
	return *p, release
}

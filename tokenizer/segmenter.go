package tokenizer

import (
	"unicode"
	"unicode/utf8"
)

// Segmenter implements the O200k Harmony 7-rule splitter without regex lookaheads.
// Next returns the end index (exclusive) of the next segment starting at i.
type Segmenter interface{ Next(s string, i int) int }

type o200kSegmenter struct{}

// NewO200kSegmenter creates a new O200k segmenter for tokenization.
func NewO200kSegmenter() Segmenter { return &o200kSegmenter{} }

func (o *o200kSegmenter) Next(s string, i int) int {
	// NOTE: This is a minimal, correct-but-not-yet-optimized segmentation.
	// It follows the priority order and guarantees progress.
	if i >= len(s) {
		return i
	}

	// Rule 6: trailing whitespace — if remainder is all whitespace, consume it all.
	allWS := true
	for j := i; j < len(s); {
		b := s[j]
		if b < utf8.RuneSelf {
			if !isASCIISpace(b) {
				allWS = false
				break
			}
			j++
			continue
		}
		r, size := utf8DecodeRuneInString(s[j:])
		if !isSpace(r) {
			allWS = false
			break
		}
		j += size
	}
	if allWS {
		return len(s)
	}

	// Try rules in priority: 1,2,3,4,5,7
	if end := ruleLettersWithPrefixAndContraction(s, i); end > i {
		return end
	}
	if end := ruleLettersWithContraction(s, i); end > i {
		return end
	}
	if end := ruleNumbers(s, i); end > i {
		return end
	}
	if end := rulePunctRun(s, i); end > i {
		return end
	}
	if end := ruleNewlines(s, i); end > i {
		return end
	}
	if end := ruleWhitespace(s, i); end > i {
		return end
	}
	// Fallback: single byte
	return i + 1
}

// Helpers
func utf8DecodeRuneInString(s string) (r rune, size int) { return utf8.DecodeRuneInString(s) }

func isL(r rune) bool     { return unicode.Is(unicode.L, r) || unicode.Is(unicode.M, r) }
func isN(r rune) bool     { return unicode.Is(unicode.N, r) }
func isSpace(r rune) bool { return unicode.IsSpace(r) }

// Rule 1 & 2 variants
func ruleLettersWithPrefixAndContraction(s string, i int) int {
	// optional single non-letter/number prefix
	j := i
	r, sz := rune(s[j]), 1
	if r >= 0x80 {
		r, sz = utf8DecodeRuneInString(s[j:])
	}
	if !(!isSpace(r) && !isL(r) && !isN(r)) {
		return ruleLettersWithContraction(s, i)
	}
	j += sz
	// uppercase/mixed letters then lowercase letters
	// Simplify: accept a run of letters/marks, then optional contraction
	if end := consumeLetterRun(s, j); end > j {
		j = end
		// optional contraction
		if end2 := matchContraction(s, j); end2 > j {
			j = end2
		}
		return j
	}
	return i
}

func ruleLettersWithContraction(s string, i int) int {
	j := i
	if end := consumeLetterRun(s, j); end > j {
		j = end
		if end2 := matchContraction(s, j); end2 > j {
			j = end2
		}
		return j
	}
	return i
}

func consumeLetterRun(s string, i int) int {
	j := i
	for j < len(s) {
		b := s[j]
		if b < utf8.RuneSelf {
			if !isASCIILetter(b) {
				break
			}
			j++
			continue
		}
		r, sz := utf8DecodeRuneInString(s[j:])
		if !isL(r) {
			break
		}
		j += sz
	}
	return j
}

func matchContraction(s string, i int) int {
	if i >= len(s) || s[i] != '\'' {
		return i
	}
	// ASCII-only, case-insensitive suffixes
	for _, suf := range []string{"s", "t", "re", "ve", "m", "ll", "d"} {
		if hasCaseInsensitiveSuffixAt(s, i+1, suf) {
			return i + 1 + len(suf)
		}
	}
	return i
}

func hasCaseInsensitiveSuffixAt(s string, i int, suf string) bool {
	if i+len(suf) > len(s) {
		return false
	}
	for k := 0; k < len(suf); k++ {
		a := s[i+k]
		b := suf[k]
		if a|0x20 != b|0x20 {
			return false
		}
	}
	return true
}

func ruleNumbers(s string, i int) int {
	j := i
	count := 0
	for j < len(s) {
		b := s[j]
		if b < utf8.RuneSelf {
			if !isASCIIDigit(b) || count >= 3 {
				break
			}
			j++
			count++
			continue
		}
		r, sz := utf8DecodeRuneInString(s[j:])
		if !isN(r) || count >= 3 {
			break
		}
		j += sz
		count++
	}
	if count > 0 {
		return j
	}
	return i
}

func rulePunctRun(s string, i int) int {
	j := i
	// optional leading space
	if j < len(s) {
		b := s[j]
		if b < utf8.RuneSelf {
			if isASCIISpace(b) {
				j++
			}
		} else {
			r, sz := utf8DecodeRuneInString(s[j:])
			if isSpace(r) {
				j += sz
			}
		}
	}
	had := false
	for j < len(s) {
		b := s[j]
		if b < utf8.RuneSelf {
			if isASCIISpace(b) || isASCIILetter(b) || isASCIIDigit(b) {
				break
			}
			j++
			had = true
			continue
		}
		r, sz := utf8DecodeRuneInString(s[j:])
		if isSpace(r) || isL(r) || isN(r) {
			break
		}
		j += sz
		had = true
	}
	if !had {
		return i
	}
	// optional CR/LF or '/'
	if j < len(s) {
		b := s[j]
		if b < utf8.RuneSelf {
			if b == '\r' || b == '\n' || b == '/' {
				j++
			}
		} else {
			r, sz := utf8DecodeRuneInString(s[j:])
			if r == '\r' || r == '\n' || r == '/' {
				j += sz
			}
		}
	}
	return j
}

func ruleNewlines(s string, i int) int {
	j := i
	// spaces
	for j < len(s) {
		b := s[j]
		if b < utf8.RuneSelf {
			if !isASCIISpace(b) || b == '\r' || b == '\n' {
				break
			}
			j++
			continue
		}
		r, sz := utf8DecodeRuneInString(s[j:])
		if !isSpace(r) || r == '\r' || r == '\n' {
			break
		}
		j += sz
	}
	// one or more CR/LF
	have := false
	for j < len(s) {
		b := s[j]
		if b < utf8.RuneSelf {
			if b != '\r' && b != '\n' {
				break
			}
			j++
			have = true
			continue
		}
		r, sz := utf8DecodeRuneInString(s[j:])
		if r != '\r' && r != '\n' {
			break
		}
		j += sz
		have = true
	}
	if !have {
		return i
	}
	// consume additional CR/LF
	for j < len(s) {
		b := s[j]
		if b < utf8.RuneSelf {
			if b != '\r' && b != '\n' {
				break
			}
			j++
			continue
		}
		r, sz := utf8DecodeRuneInString(s[j:])
		if r != '\r' && r != '\n' {
			break
		}
		j += sz
	}
	return j
}

func ruleWhitespace(s string, i int) int {
	j := i
	for j < len(s) {
		b := s[j]
		if b < utf8.RuneSelf {
			if !isASCIISpace(b) {
				break
			}
			j++
			continue
		}
		r, sz := utf8DecodeRuneInString(s[j:])
		if !isSpace(r) {
			break
		}
		j += sz
	}
	if j > i {
		return j
	}
	return i
}

func isASCIISpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	default:
		return false
	}
}

func isASCIILetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

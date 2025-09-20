package tokenizer

import "testing"

func TestSegmenterASCIIEquivalence(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		expect []string
	}{
		{
			name:   "letters and spaces",
			text:   "hello   world",
			expect: []string{"hello", "   ", "world"},
		},
		{
			name:   "numbers limited to three",
			text:   "1234abc",
			expect: []string{"123", "4", "abc"},
		},
		{
			name:   "letters numbers mix",
			text:   "abc1234",
			expect: []string{"abc", "123", "4"},
		},
		{
			name:   "punctuation run",
			text:   "foo!!!/bar",
			expect: []string{"foo", "!!!/", "bar"},
		},
		{
			name:   "spaces and newlines",
			text:   "  \n\nabc",
			expect: []string{"  \n\n", "abc"},
		},
		{
			name:   "all whitespace",
			text:   "\t \n",
			expect: []string{"\t \n"},
		},
	}

	s := NewO200kSegmenter()
	for _, tc := range tests {
		segments := collectSegments(s, tc.text)
		if len(segments) != len(tc.expect) {
			t.Fatalf("%s: segment count %d want %d (%v)", tc.name, len(segments), len(tc.expect), segments)
		}
		for i := range segments {
			if segments[i] != tc.expect[i] {
				t.Fatalf("%s: segment %d = %q want %q", tc.name, i, segments[i], tc.expect[i])
			}
		}
		// Cross-check with slow implementation for ASCII strings
		ref := collectSegments(slowSegmenter{}, tc.text)
		if len(ref) != len(segments) {
			t.Fatalf("%s: slow segments %v fast %v", tc.name, ref, segments)
		}
		for i := range ref {
			if ref[i] != segments[i] {
				t.Fatalf("%s: slow segment %d = %q fast %q", tc.name, i, ref[i], segments[i])
			}
		}
	}
}

func collectSegments(seg Segmenter, text string) []string {
	var out []string
	for i := 0; i < len(text); {
		next := seg.Next(text, i)
		if next <= i {
			panic("segmenter did not advance")
		}
		out = append(out, text[i:next])
		i = next
	}
	if len(text) == 0 {
		return out
	}
	return out
}

// slowSegmenter mirrors the pre-optimization Unicode-driven logic for ASCII verification.
type slowSegmenter struct{}

func (slowSegmenter) Next(s string, i int) int {
	if i >= len(s) {
		return i
	}
	allWS := true
	for j := i; j < len(s); {
		r, size := rune(s[j]), 1
		if r >= 0x80 {
			r, size = utf8DecodeRuneInString(s[j:])
		}
		if !isSpace(r) {
			allWS = false
			break
		}
		j += size
	}
	if allWS {
		return len(s)
	}
	if end := slowRuleLettersWithPrefixAndContraction(s, i); end > i {
		return end
	}
	if end := slowRuleLettersWithContraction(s, i); end > i {
		return end
	}
	if end := slowRuleNumbers(s, i); end > i {
		return end
	}
	if end := slowRulePunctRun(s, i); end > i {
		return end
	}
	if end := slowRuleNewlines(s, i); end > i {
		return end
	}
	if end := slowRuleWhitespace(s, i); end > i {
		return end
	}
	return i + 1
}

func slowRuleLettersWithPrefixAndContraction(s string, i int) int {
	j := i
	r, sz := rune(s[j]), 1
	if r >= 0x80 {
		r, sz = utf8DecodeRuneInString(s[j:])
	}
	if !(!isSpace(r) && !isL(r) && !isN(r)) {
		return slowRuleLettersWithContraction(s, i)
	}
	j += sz
	if end := slowConsumeLetterRun(s, j); end > j {
		j = end
		if end2 := matchContraction(s, j); end2 > j {
			j = end2
		}
		return j
	}
	return i
}

func slowRuleLettersWithContraction(s string, i int) int {
	j := i
	if end := slowConsumeLetterRun(s, j); end > j {
		j = end
		if end2 := matchContraction(s, j); end2 > j {
			j = end2
		}
		return j
	}
	return i
}

func slowConsumeLetterRun(s string, i int) int {
	j := i
	for j < len(s) {
		r, sz := rune(s[j]), 1
		if r >= 0x80 {
			r, sz = utf8DecodeRuneInString(s[j:])
		}
		if !isL(r) {
			break
		}
		j += sz
	}
	return j
}

func slowRuleNumbers(s string, i int) int {
	j := i
	count := 0
	for j < len(s) {
		r, sz := rune(s[j]), 1
		if r >= 0x80 {
			r, sz = utf8DecodeRuneInString(s[j:])
		}
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

func slowRulePunctRun(s string, i int) int {
	j := i
	if j < len(s) {
		r, sz := rune(s[j]), 1
		if r >= 0x80 {
			r, sz = utf8DecodeRuneInString(s[j:])
		}
		if isSpace(r) {
			j += sz
		}
	}
	had := false
	for j < len(s) {
		r, sz := rune(s[j]), 1
		if r >= 0x80 {
			r, sz = utf8DecodeRuneInString(s[j:])
		}
		if isSpace(r) || isL(r) || isN(r) {
			break
		}
		j += sz
		had = true
	}
	if !had {
		return i
	}
	if j < len(s) {
		r, sz := rune(s[j]), 1
		if r >= 0x80 {
			r, sz = utf8DecodeRuneInString(s[j:])
		}
		if r == '\r' || r == '\n' || r == '/' {
			j += sz
		}
	}
	return j
}

func slowRuleNewlines(s string, i int) int {
	j := i
	for j < len(s) {
		r, sz := rune(s[j]), 1
		if r >= 0x80 {
			r, sz = utf8DecodeRuneInString(s[j:])
		}
		if !isSpace(r) || r == '\r' || r == '\n' {
			break
		}
		j += sz
	}
	have := false
	for j < len(s) {
		r, sz := rune(s[j]), 1
		if r >= 0x80 {
			r, sz = utf8DecodeRuneInString(s[j:])
		}
		if r != '\r' && r != '\n' {
			break
		}
		j += sz
		have = true
	}
	if !have {
		return i
	}
	for j < len(s) {
		r, sz := rune(s[j]), 1
		if r >= 0x80 {
			r, sz = utf8DecodeRuneInString(s[j:])
		}
		if r != '\r' && r != '\n' {
			break
		}
		j += sz
	}
	return j
}

func slowRuleWhitespace(s string, i int) int {
	j := i
	for j < len(s) {
		r, sz := rune(s[j]), 1
		if r >= 0x80 {
			r, sz = utf8DecodeRuneInString(s[j:])
		}
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

package tokenizer

// Public thin wrappers to keep package boundary small.

// Core is an alias exposing exported methods defined on coreBPE.
type Core = coreBPE

// NewCoreBPE creates a new Core BPE tokenizer with the given pairs, special tokens, and segmenter.
func NewCoreBPE(pairs [][2]any, specials map[string]uint32, seg Segmenter) (*Core, error) {
	return newCoreBPE(pairs, specials, seg)
}

// HarmonySpecials returns the default special tokens used by Harmony tokenizers.
func HarmonySpecials() map[string]uint32 { return buildHarmonySpecials() }

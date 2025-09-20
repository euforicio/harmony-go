package tokenizer

import "fmt"

// Harmony special token ids and reserved ranges (must exactly match the upstream spec).
const (
	TokStartOfText uint32 = 199998
	TokEndOfText   uint32 = 199999

	TokReturn    uint32 = 200002
	TokConstrain uint32 = 200003
	TokChannel   uint32 = 200005
	TokStart     uint32 = 200006
	TokEnd       uint32 = 200007
	TokMessage   uint32 = 200008
	TokCall      uint32 = 200012
)

// Reserved range for Harmony: 200014..=201088
const (
	ReservedStart = 200014
	ReservedEnd   = 201088
)

func buildHarmonySpecials() map[string]uint32 {
	m := map[string]uint32{
		"<|startoftext|>": TokStartOfText,
		"<|endoftext|>":   TokEndOfText,
		"<|return|>":      TokReturn,
		"<|constrain|>":   TokConstrain,
		"<|channel|>":     TokChannel,
		"<|start|>":       TokStart,
		"<|end|>":         TokEnd,
		"<|message|>":     TokMessage,
		"<|call|>":        TokCall,
	}
	// Reserved mapping
	for id := uint32(ReservedStart); id <= uint32(ReservedEnd); id++ {
		key := fmt.Sprintf("<|reserved_%d|>", id)
		m[key] = id
	}
	return m
}

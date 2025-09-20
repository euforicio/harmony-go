package harmony

import "testing"

func TestStreamParserGetters(t *testing.T) {
	enc, err := LoadEncoding(HarmonyGptOss)
	if err != nil {
		t.Fatal(err)
	}
	// Build a simple assistant message: "Hello"
	text := "<|start|>assistant<|message|>Hello<|end|>"
	toks := enc.bpe.EncodeWithSpecialTokens(text)

	p, err := NewStreamParser(enc, nil)
	if err != nil {
		t.Fatal(err)
	}
	seenDelta := false
	for _, tk := range toks {
		if err := p.Process(tk); err != nil {
			t.Fatal(err)
		}
		if d := p.LastContentDelta(); d != "" {
			seenDelta = true
		}
	}
	if !seenDelta {
		t.Fatalf("expected to observe at least one content delta")
	}

	// After full parse we should have one message
	if len(p.Messages()) != 1 {
		t.Fatalf("expected 1 message, got %d", len(p.Messages()))
	}

	// Getters
	if role := p.CurrentRole(); role != nil {
		t.Fatalf("expected nil role after finalization, got %v", *role)
	}
	if p.CurrentContent() != "" {
		t.Fatalf("expected empty current content after finalization")
	}
}

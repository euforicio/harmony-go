package harmony

import (
	"strings"
	"testing"

	"slices"

	"github.com/euforicio/harmony-go/tokenizer"
)

func mustEncoding(t *testing.T) *Encoding {
	t.Helper()
	enc, err := LoadEncoding(HarmonyGptOss)
	if err != nil {
		t.Fatalf("LoadEncoding: %v", err)
	}
	return enc
}

func TestStopTokens(t *testing.T) {
	enc := mustEncoding(t)

	got, err := enc.StopTokens()
	if err != nil {
		t.Fatalf("StopTokens: %v", err)
	}
	slices.Sort(got)
	want := []uint32{tokenizer.TokCall, tokenizer.TokEnd, tokenizer.TokReturn}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("StopTokens mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestStopTokensForAssistantActions(t *testing.T) {
	enc := mustEncoding(t)

	got, err := enc.StopTokensForAssistantActions()
	if err != nil {
		t.Fatalf("StopTokensForAssistantActions: %v", err)
	}
	slices.Sort(got)
	want := []uint32{tokenizer.TokCall, tokenizer.TokReturn}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("StopTokensForAssistantActions mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestRenderConversationForCompletion(t *testing.T) {
	enc := mustEncoding(t)

	conv := Conversation{Messages: []Message{
		{
			Author:  Author{Role: RoleUser},
			Content: []Content{{Type: ContentText, Text: "ping"}},
		},
		{
			Author:  Author{Role: RoleAssistant},
			Channel: "final",
			Content: []Content{{Type: ContentText, Text: "pong"}},
		},
	}}

	base, err := enc.RenderConversation(conv, nil)
	if err != nil {
		t.Fatalf("RenderConversation: %v", err)
	}
	withSuffix, err := enc.RenderConversationForCompletion(conv, RoleAssistant, nil)
	if err != nil {
		t.Fatalf("RenderConversationForCompletion: %v", err)
	}
	if len(withSuffix) != len(base)+len(enc.EncodeWithSpecialTokens(string(RoleAssistant)))+1 {
		t.Fatalf("unexpected completion length: base=%d suffix len=%d got=%d", len(base), len(enc.EncodeWithSpecialTokens(string(RoleAssistant)))+1, len(withSuffix))
	}
	if !slices.Equal(withSuffix[:len(base)], base) {
		t.Fatalf("conversation prefix changed during completion render")
	}
	expectedSuffix := append([]uint32{tokenizer.TokStart}, enc.EncodeWithSpecialTokens(string(RoleAssistant))...)
	if !slices.Equal(withSuffix[len(base):], expectedSuffix) {
		t.Fatalf("completion suffix mismatch\n got: %v\nwant: %v", withSuffix[len(base):], expectedSuffix)
	}
}

func TestRenderConversationForTraining(t *testing.T) {
	enc := mustEncoding(t)

	conv := Conversation{Messages: []Message{
		{
			Author:  Author{Role: RoleUser},
			Content: []Content{{Type: ContentText, Text: "ping"}},
		},
		{
			Author:  Author{Role: RoleAssistant},
			Channel: "final",
			Content: []Content{{Type: ContentText, Text: "pong"}},
		},
	}}

	base, err := enc.RenderConversation(conv, nil)
	if err != nil {
		t.Fatalf("RenderConversation: %v", err)
	}
	training, err := enc.RenderConversationForTraining(conv, nil)
	if err != nil {
		t.Fatalf("RenderConversationForTraining: %v", err)
	}
	if len(training) != len(base) {
		t.Fatalf("expected training tokens to match base length: base=%d training=%d", len(base), len(training))
	}
	if training[len(training)-1] != tokenizer.TokReturn {
		t.Fatalf("expected trailing token to be <|return|>, got %d", training[len(training)-1])
	}
	if base[len(base)-1] != tokenizer.TokEnd {
		t.Fatalf("expected base render to end with <|end|>, got %d", base[len(base)-1])
	}
	if !slices.Equal(training[:len(training)-1], base[:len(base)-1]) {
		t.Fatalf("training render should only differ in final token")
	}

	// Non-final assistant should remain unchanged.
	plainConv := Conversation{Messages: []Message{
		{
			Author:  Author{Role: RoleUser},
			Content: []Content{{Type: ContentText, Text: "ping"}},
		},
		{
			Author:  Author{Role: RoleAssistant},
			Channel: "analysis",
			Content: []Content{{Type: ContentText, Text: "thinking"}},
		},
	}}
	plainBase, err := enc.RenderConversation(plainConv, nil)
	if err != nil {
		t.Fatalf("RenderConversation plain: %v", err)
	}
	plainTraining, err := enc.RenderConversationForTraining(plainConv, nil)
	if err != nil {
		t.Fatalf("RenderConversationForTraining plain: %v", err)
	}
	if !slices.Equal(plainBase, plainTraining) {
		t.Fatalf("expected non-final training render to match base\n base: %v\ntrain: %v", plainBase, plainTraining)
	}
}

func TestRenderContentTypeConstrain(t *testing.T) {
	enc := mustEncoding(t)
	msg := Message{
		Author:      Author{Role: RoleAssistant},
		ContentType: "<|constrain|>json",
		Content:     []Content{{Type: ContentText, Text: "{}"}},
	}

	toks, err := enc.Render(msg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	messageIdx := slices.Index(toks, tokenizer.TokMessage)
	if messageIdx == -1 {
		t.Fatalf("render output missing <|message|>")
	}
	spaceTokens := enc.EncodeWithSpecialTokens(" ")
	restTokens := enc.EncodeWithSpecialTokens("json")
	expected := append(append(append([]uint32{}, spaceTokens...), tokenizer.TokConstrain), restTokens...)
	start := messageIdx - len(expected)
	if start < 0 {
		t.Fatalf("not enough tokens before <|message|> to hold content type")
	}
	if !slices.Equal(toks[start:messageIdx], expected) {
		t.Fatalf("content-type tokens mismatch\n got: %v\nwant: %v", toks[start:messageIdx], expected)
	}
}

func TestRenderContentTypePlain(t *testing.T) {
	enc := mustEncoding(t)
	msg := Message{
		Author:      Author{Role: RoleAssistant},
		ContentType: "text/plain",
		Content:     []Content{{Type: ContentText, Text: "ok"}},
	}

	toks, err := enc.Render(msg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	messageIdx := slices.Index(toks, tokenizer.TokMessage)
	if messageIdx == -1 {
		t.Fatalf("render output missing <|message|>")
	}
	expected := enc.EncodeWithSpecialTokens(" text/plain")
	start := messageIdx - len(expected)
	if start < 0 {
		t.Fatalf("not enough tokens before <|message|> to hold content type")
	}
	if !slices.Equal(toks[start:messageIdx], expected) {
		t.Fatalf("plain content-type tokens mismatch\n got: %v\nwant: %v", toks[start:messageIdx], expected)
	}
}

func TestRenderConversationAutoDropAnalysis(t *testing.T) {
	enc := mustEncoding(t)

	conv := Conversation{Messages: []Message{
		{
			Author:  Author{Role: RoleUser},
			Content: []Content{{Type: ContentText, Text: "hi"}},
		},
		{
			Author:  Author{Role: RoleAssistant},
			Channel: "analysis",
			Content: []Content{{Type: ContentText, Text: "thinking"}},
		},
		{
			Author:  Author{Role: RoleAssistant},
			Channel: "commentary",
			Content: []Content{{Type: ContentText, Text: "call tool"}},
		},
		{
			Author:  Author{Role: RoleAssistant},
			Channel: "final",
			Content: []Content{{Type: ContentText, Text: "done"}},
		},
	}}

	// Default behaviour drops analysis messages before the first final.
	baselineTokens, err := enc.RenderConversation(conv, nil)
	if err != nil {
		t.Fatalf("RenderConversation auto-drop: %v", err)
	}
	msgs, err := enc.ParseMessagesFromCompletionTokens(baselineTokens, nil)
	if err != nil {
		t.Fatalf("ParseMessagesFromCompletionTokens auto-drop: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages after auto-drop, got %d", len(msgs))
	}
	if msgs[1].Channel != "commentary" {
		t.Fatalf("expected commentary message at index 1, got channel %q", msgs[1].Channel)
	}
	if msgs[1].Content[0].Text != "call tool" {
		t.Fatalf("dropped conversation altered commentary text: %q", msgs[1].Content[0].Text)
	}
	for _, m := range msgs {
		if m.Channel == "analysis" {
			t.Fatalf("analysis message should have been dropped: %+v", m)
		}
	}

	// Disabling auto-drop retains the analysis message.
	cfg := &RenderConversationConfig{AutoDropAnalysis: false}
	noDropTokens, err := enc.RenderConversation(conv, cfg)
	if err != nil {
		t.Fatalf("RenderConversation no-drop: %v", err)
	}
	noDropMsgs, err := enc.ParseMessagesFromCompletionTokens(noDropTokens, nil)
	if err != nil {
		t.Fatalf("ParseMessagesFromCompletionTokens no-drop: %v", err)
	}
	if len(noDropMsgs) != 4 {
		t.Fatalf("expected 4 messages without auto-drop, got %d", len(noDropMsgs))
	}
	if noDropMsgs[1].Channel != "analysis" {
		t.Fatalf("analysis message missing when auto-drop disabled, channel %q", noDropMsgs[1].Channel)
	}
}

func TestRenderConversationParallelDeterminism(t *testing.T) {
	enc := mustEncoding(t)
	large := strings.Repeat("All work and no play makes Jack a dull boy. ", 200)
	conv := Conversation{Messages: []Message{
		{
			Author:  Author{Role: RoleUser},
			Content: []Content{{Type: ContentText, Text: large}},
		},
		{
			Author:  Author{Role: RoleAssistant},
			Channel: "commentary",
			Content: []Content{{Type: ContentText, Text: large}},
		},
	}}

	if len(conv.Messages) < 2 {
		t.Fatalf("conversation must contain at least two messages")
	}

	// Sequential baseline via per-message rendering.
	var sequential []uint32
	for _, msg := range conv.Messages {
		toks, err := enc.renderMessage(msg, renderOptions{})
		if err != nil {
			t.Fatalf("renderMessage: %v", err)
		}
		sequential = append(sequential, toks...)
	}

	parallelTokens, err := enc.RenderConversation(conv, &RenderConversationConfig{AutoDropAnalysis: false})
	if err != nil {
		t.Fatalf("RenderConversation parallel: %v", err)
	}
	if len(parallelTokens) < 1000 {
		t.Fatalf("expected large token output for parallel path, got %d tokens", len(parallelTokens))
	}
	if !slices.Equal(parallelTokens, sequential) {
		t.Fatalf("parallel render differed from sequential baseline")
	}
}

package harmony

import (
	"testing"

	"github.com/euforicio/harmony-go/tokenizer"
)

func expectState(t *testing.T, p *StreamParser, want string) {
	t.Helper()
	got, err := p.StateJSON()
	if err != nil {
		t.Fatalf("StateJSON: %v", err)
	}
	if got != want {
		t.Fatalf("state mismatch: got %s want %s", got, want)
	}
}

func TestStreamParserAccessors(t *testing.T) {
	enc := mustEncoding(t)

	content := "{\"foo\":1}"
	msg := Message{
		Author:      Author{Role: RoleAssistant, Name: "scribe"},
		Recipient:   "user",
		Channel:     "analysis",
		ContentType: "<|constrain|>json",
		Content:     []Content{{Type: ContentText, Text: content}},
	}

	tokens, err := enc.Render(msg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	parser, err := NewStreamParser(enc, nil)
	if err != nil {
		t.Fatalf("NewStreamParser: %v", err)
	}

	expectState(t, parser, `{"state":"ExpectStart"}`)

	if err := parser.Process(tokens[0]); err != nil {
		t.Fatalf("Process start: %v", err)
	}
	expectState(t, parser, `{"state":"Header"}`)

	idx := 1
	for ; idx < len(tokens); idx++ {
		tok := tokens[idx]
		if err := parser.Process(tok); err != nil {
			t.Fatalf("Process header token %d: %v", idx, err)
		}
		if tok == tokenizer.TokMessage {
			expectState(t, parser, `{"state":"Content"}`)
			break
		}
		expectState(t, parser, `{"state":"Header"}`)
	}
	if idx >= len(tokens) {
		t.Fatalf("TokMessage not encountered in stream")
	}

	role := parser.CurrentRole()
	if role == nil || *role != RoleAssistant {
		t.Fatalf("CurrentRole = %v, want assistant", role)
	}
	if got := parser.CurrentChannel(); got != "analysis" {
		t.Fatalf("CurrentChannel = %q, want analysis", got)
	}
	if got := parser.CurrentRecipient(); got != "user" {
		t.Fatalf("CurrentRecipient = %q, want user", got)
	}
	if got := parser.CurrentContentType(); got != "<|constrain|>json" {
		t.Fatalf("CurrentContentType = %q", got)
	}
	if got := parser.CurrentContent(); got != "" {
		t.Fatalf("CurrentContent should be empty before content tokens, got %q", got)
	}

	stopIdx := -1
	for j := idx + 1; j < len(tokens); j++ {
		tok := tokens[j]
		if _, isStop := enc.stopAll[tok]; isStop {
			stopIdx = j
			break
		}
		if err := parser.Process(tok); err != nil {
			t.Fatalf("Process content token %d: %v", j, err)
		}
		expectState(t, parser, `{"state":"Content"}`)
		if parser.LastContentDelta() == "" {
			t.Fatalf("LastContentDelta empty after consuming content token %d", j)
		}
	}
	if stopIdx == -1 {
		t.Fatalf("stop token not found in stream")
	}

	if got := parser.CurrentContent(); got != content {
		t.Fatalf("CurrentContent = %q, want %q", got, content)
	}

	if err := parser.Process(tokens[stopIdx]); err != nil {
		t.Fatalf("Process stop token: %v", err)
	}
	expectState(t, parser, `{"state":"ExpectStart"}`)
	if got := parser.CurrentChannel(); got != "" {
		t.Fatalf("CurrentChannel should reset after stop, got %q", got)
	}
	if got := parser.CurrentRecipient(); got != "" {
		t.Fatalf("CurrentRecipient should reset after stop, got %q", got)
	}
	if got := parser.CurrentContentType(); got != "" {
		t.Fatalf("CurrentContentType should reset after stop, got %q", got)
	}

	if err := parser.ProcessEOS(); err != nil {
		t.Fatalf("ProcessEOS: %v", err)
	}

	msgs := parser.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 parsed message, got %d", len(msgs))
	}
	gotMsg := msgs[0]
	if gotMsg.Author.Role != RoleAssistant || gotMsg.Author.Name != "scribe" {
		t.Fatalf("parsed author mismatch: %+v", gotMsg.Author)
	}
	if gotMsg.Recipient != "user" {
		t.Fatalf("parsed recipient = %q", gotMsg.Recipient)
	}
	if gotMsg.Channel != "analysis" {
		t.Fatalf("parsed channel = %q", gotMsg.Channel)
	}
	if gotMsg.ContentType != "<|constrain|>json" {
		t.Fatalf("parsed content type = %q", gotMsg.ContentType)
	}
	if len(gotMsg.Content) != 1 || gotMsg.Content[0].Text != content {
		t.Fatalf("parsed content mismatch: %+v", gotMsg.Content)
	}

	if tokensLen := len(tokens); tokensLen != len(parser.Tokens()) {
		t.Fatalf("Tokens length mismatch: render=%d parser=%d", tokensLen, len(parser.Tokens()))
	}
}

package harmony

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/euforicio/harmony-go/tokenizer"
)

func strPtr(s string) *string { return &s }

func reasoningPtr(r ReasoningEffort) *ReasoningEffort { return &r }

func extractMessageBody(t *testing.T, enc *Encoding, tokens []uint32, messageStart int) string {
	t.Helper()
	msgIdx := -1
	for i := messageStart; i < len(tokens); i++ {
		if tokens[i] == tokenizer.TokMessage {
			msgIdx = i
			break
		}
	}
	if msgIdx == -1 {
		t.Fatalf("TokMessage not found from index %d", messageStart)
	}
	endIdx := -1
	for i := msgIdx + 1; i < len(tokens); i++ {
		if tokens[i] == tokenizer.TokEnd || tokens[i] == tokenizer.TokCall {
			endIdx = i
			break
		}
	}
	if endIdx == -1 {
		t.Fatalf("message end token not found after index %d", msgIdx)
	}
	bodyTokens := tokens[msgIdx+1 : endIdx]
	text, err := enc.DecodeUTF8(bodyTokens)
	if err != nil {
		t.Fatalf("DecodeUTF8: %v", err)
	}
	return text
}

func TestRenderSystemContent_Text(t *testing.T) {
	enc := mustEncoding(t)

	model := "Harmony Model Pro"
	cutoff := "2024-09"
	current := "2025-09-21"
	reasoning := reasoningPtr(ReasoningHigh)

	sysContent := SystemContent{
		ModelIdentity:         &model,
		KnowledgeCutoff:       &cutoff,
		ConversationStartDate: &current,
		ReasoningEffort:       reasoning,
		ChannelConfig: &ChannelConfig{
			ValidChannels:   []string{"analysis", "commentary", "final"},
			ChannelRequired: true,
		},
	}

	devTools := DeveloperContent{
		Tools: map[string]ToolNamespaceConfig{
			"functions": {
				Name: "functions",
				Tools: []ToolDescription{{
					Name:        "noop",
					Description: "placeholder",
				}},
			},
		},
	}

	conv := Conversation{Messages: []Message{
		{
			Author:  Author{Role: RoleSystem},
			Channel: "system",
			Content: []Content{{Type: ContentSystem, System: &sysContent}},
		},
		{
			Author:  Author{Role: RoleDeveloper},
			Channel: "commentary",
			Content: []Content{{Type: ContentDeveloper, Developer: &devTools}},
		},
	}}

	tokens, err := enc.RenderConversation(conv, nil)
	if err != nil {
		t.Fatalf("RenderConversation: %v", err)
	}

	if tokens[0] != tokenizer.TokStart {
		t.Fatalf("first token = %d, want TokStart", tokens[0])
	}
	messageBody := extractMessageBody(t, enc, tokens, 0)

	if !strings.Contains(messageBody, model) {
		t.Fatalf("system content missing model identity: %q", messageBody)
	}
	if !strings.Contains(messageBody, "Knowledge cutoff: "+cutoff) {
		t.Fatalf("system content missing knowledge cutoff: %q", messageBody)
	}
	if !strings.Contains(messageBody, "Current date: "+current) {
		t.Fatalf("system content missing current date: %q", messageBody)
	}
	if !strings.Contains(messageBody, "Reasoning: high") {
		t.Fatalf("system content missing reasoning line: %q", messageBody)
	}
	if !strings.Contains(messageBody, "# Valid channels: analysis, commentary, final.") {
		t.Fatalf("system content missing valid channels line: %q", messageBody)
	}
	functionsLine := "Calls to these tools must go to the commentary channel: 'functions'."
	if !strings.Contains(messageBody, functionsLine) {
		t.Fatalf("system content missing functions routing note: %q", messageBody)
	}

	msgIdx := strings.Index(messageBody, model)
	if msgIdx == -1 {
		t.Fatalf("model identity not present for position check")
	}

	// Ensure channel marker tokens are present before <|message|>.
	channelIdx := -1
	for i, tok := range tokens {
		if tok == tokenizer.TokChannel {
			channelIdx = i
			break
		}
		if tok == tokenizer.TokMessage {
			break
		}
	}
	if channelIdx == -1 {
		t.Fatalf("expected TokChannel before TokMessage in header")
	}
	channelTextTokens := tokens[channelIdx+1:]
	msgTokenPos := -1
	for i, tok := range channelTextTokens {
		if tok == tokenizer.TokMessage {
			msgTokenPos = i
			break
		}
	}
	if msgTokenPos == -1 {
		t.Fatalf("TokMessage not found after channel tokens")
	}
	channelDecoded, err := enc.DecodeUTF8(channelTextTokens[:msgTokenPos])
	if err != nil {
		t.Fatalf("DecodeUTF8 channel: %v", err)
	}
	if channelDecoded != "system" {
		t.Fatalf("channel text = %q, want 'system'", channelDecoded)
	}
}

func TestRenderDeveloperContentAndTools_Text(t *testing.T) {
	enc := mustEncoding(t)

	instructions := "Use tools when helpful."
	toolDesc := "Returns weather data for a city."

	params := map[string]any{
		"type":        "object",
		"description": "Fetch weather data",
		"properties": map[string]any{
			"location": map[string]any{
				"type":        "string",
				"description": "City name",
				"default":     "San Francisco",
			},
			"unit": map[string]any{
				"type":    "string",
				"enum":    []any{"celsius", "fahrenheit"},
				"default": "celsius",
			},
			"mode": map[string]any{
				"description": "Select variant",
				"oneOf": []any{
					map[string]any{
						"type":        "string",
						"enum":        []any{"current"},
						"description": "Current weather",
					},
					map[string]any{
						"type":        "string",
						"enum":        []any{"forecast"},
						"description": "Forecast weather",
						"default":     "forecast",
					},
				},
			},
		},
		"required": []any{"location"},
	}

	rawParams, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal params: %v", err)
	}

	msg := Message{
		Author: Author{Role: RoleDeveloper},
		Content: []Content{{
			Type: ContentDeveloper,
			Developer: &DeveloperContent{
				Instructions: &instructions,
				Tools: map[string]ToolNamespaceConfig{
					"functions": {
						Name:        "functions",
						Description: strPtr("Function calls allowed."),
						Tools: []ToolDescription{{
							Name:        "callWeather",
							Description: toolDesc,
							Parameters:  rawParams,
						}},
					},
				},
			},
		}},
	}

	tokens, err := enc.Render(msg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	body := extractMessageBody(t, enc, tokens, 0)

	checks := []string{
		"# Instructions\n\n" + instructions,
		"# Tools",
		"## functions",
		"// " + toolDesc,
		"namespace functions {",
		"type callWeather = (_: // Fetch weather data\n{",
		"location: string, // default: \"San Francisco\"",
		"unit?: \"celsius\" | \"fahrenheit\", // default: celsius",
		"mode?:",
		"| \"current\"",
		"| \"forecast\" // Forecast weather default: forecast",
	}
	for _, sub := range checks {
		if !strings.Contains(body, sub) {
			t.Fatalf("developer content missing %q in body:\n%s", sub, body)
		}
	}
}

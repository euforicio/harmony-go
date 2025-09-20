package harmony

// ExampleEncoding_Render shows how to render a single message to Harmony tokens.
func ExampleEncoding_Render() {
	enc, _ := LoadEncoding(HarmonyGptOss)
	msg := Message{
		Author:  Author{Role: RoleUser},
		Content: []Content{{Type: ContentText, Text: "ping"}},
	}
	_, _ = enc.Render(msg)
	// Output:
}

// ExampleEncoding_ParseMessagesFromCompletionTokens shows how to convert
// tokens back into structured messages.
func ExampleEncoding_ParseMessagesFromCompletionTokens() {
	enc, _ := LoadEncoding(HarmonyGptOss)

	conv := Conversation{Messages: []Message{
		{Author: Author{Role: RoleUser}, Content: []Content{{Type: ContentText, Text: "ping"}}},
		{Author: Author{Role: RoleAssistant}, Channel: "final", Content: []Content{{Type: ContentText, Text: "pong"}}},
	}}

	toks, _ := enc.RenderConversation(conv, nil)
	_, _ = enc.ParseMessagesFromCompletionTokens(toks, nil)
	// Output:
}

package benchmarks

import (
	"fmt"
	"strings"

	"github.com/euforicio/harmony-go"
)

// LargeConversation constructs a synthetic conversation with sizeable messages
// to exercise the parallel rendering path.
func LargeConversation() harmony.Conversation {
	bigBlock := strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Vestibulum vulputate. ", 200)
	sys := harmony.SystemContent{
		ModelIdentity:         strPtr("Test model"),
		KnowledgeCutoff:       strPtr("2024-06"),
		ConversationStartDate: strPtr("2025-09-20"),
	}
	devInstr := strings.Repeat("Follow tool schema precisely. ", 100)
	convo := harmony.Conversation{Messages: []harmony.Message{
		{
			Author:  harmony.Author{Role: harmony.RoleSystem},
			Content: []harmony.Content{{Type: harmony.ContentSystem, System: &sys}},
		},
		{
			Author:  harmony.Author{Role: harmony.RoleDeveloper},
			Content: []harmony.Content{{Type: harmony.ContentDeveloper, Developer: &harmony.DeveloperContent{Instructions: &devInstr}}},
			Channel: "analysis",
		},
	}}

	for i := 0; i < 8; i++ {
		convo.Messages = append(convo.Messages, harmony.Message{
			Author:  harmony.Author{Role: harmony.RoleUser},
			Content: []harmony.Content{{Type: harmony.ContentText, Text: fmt.Sprintf("User block %d: %s", i, bigBlock)}},
		})
		convo.Messages = append(convo.Messages, harmony.Message{
			Author:  harmony.Author{Role: harmony.RoleAssistant},
			Channel: "analysis",
			Content: []harmony.Content{{Type: harmony.ContentText, Text: fmt.Sprintf("Assistant analysis %d: %s", i, bigBlock)}},
		})
	}
	convo.Messages = append(convo.Messages, harmony.Message{
		Author:  harmony.Author{Role: harmony.RoleAssistant},
		Channel: "final",
		Content: []harmony.Content{{Type: harmony.ContentText, Text: "Final answer summarizing the conversation."}},
	})
	return convo
}

func strPtr(s string) *string { return &s }

package harmony

import (
	"encoding/json"
)

// Role identifies the author class of a message in a Harmony conversation.
// It matches the Harmony prompt format (user, assistant, system, developer, tool).
type Role string

// Well-known roles supported by the Harmony format.
const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleDeveloper Role = "developer"
	RoleTool      Role = "tool"
)

// Author holds the message author role and optional name (e.g. a tool id).
type Author struct {
	Role Role   `json:"role"`
	Name string `json:"name,omitempty"`
}

// ReasoningEffort expresses the desired level of reasoning for the model.
type ReasoningEffort string

// Reasoning effort values.
const (
	ReasoningLow    ReasoningEffort = "low"
	ReasoningMedium ReasoningEffort = "medium"
	ReasoningHigh   ReasoningEffort = "high"
)

// ChannelConfig configures valid channels and whether a channel is required.
type ChannelConfig struct {
	ValidChannels   []string `json:"valid_channels"`
	ChannelRequired bool     `json:"channel_required"`
}

// ToolDescription describes an individual tool and its JSON Schema parameters.
type ToolDescription struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	// parsed caches derive from Parameters; kept behind a pointer so copying
	// ToolDescription values does not copy synchronization primitives.
	parsed *toolParsedCache `json:"-"`
}

// ToolNamespaceConfig groups multiple tools under a namespace (e.g. "functions").
type ToolNamespaceConfig struct {
	Name        string            `json:"name"`
	Description *string           `json:"description,omitempty"`
	Tools       []ToolDescription `json:"tools"`
}

// SystemContent encodes system instructions and metadata for the conversation.
type SystemContent struct {
	ModelIdentity         *string                        `json:"model_identity,omitempty"`
	ReasoningEffort       *ReasoningEffort               `json:"reasoning_effort,omitempty"`
	Tools                 map[string]ToolNamespaceConfig `json:"tools,omitempty"`
	ConversationStartDate *string                        `json:"conversation_start_date,omitempty"`
	KnowledgeCutoff       *string                        `json:"knowledge_cutoff,omitempty"`
	ChannelConfig         *ChannelConfig                 `json:"channel_config,omitempty"`
}

// DeveloperContent carries developer instructions and tool declarations.
type DeveloperContent struct {
	Instructions *string                        `json:"instructions,omitempty"`
	Tools        map[string]ToolNamespaceConfig `json:"tools,omitempty"`
}

// ContentType enumerates renderable content kinds in a message.
type ContentType string

// Available content kinds: plain text, system and developer content.
const (
	ContentText      ContentType = "text"
	ContentSystem    ContentType = "system_content"
	ContentDeveloper ContentType = "developer_content"
)

// Content holds a single content item within a Message.
// When Type is text, Text is set; when system or developer, the corresponding
// pointer is populated.
type Content struct {
	Type      ContentType       `json:"type"`
	Text      string            `json:"text,omitempty"`
	System    *SystemContent    `json:"system_content,omitempty"`
	Developer *DeveloperContent `json:"developer_content,omitempty"`
}

// Message represents a single Harmony message. Content is either a string or
// a list of structured Content items in JSON. Author is flattened as role/name.
// Message.content is string or []Content in JSON; we implement custom codec.
type Message struct {
	Author      Author    `json:"role"` // Flattened: role + optional name
	Recipient   string    `json:"recipient,omitempty"`
	Content     []Content `json:"content"`
	Channel     string    `json:"channel,omitempty"`
	ContentType string    `json:"content_type,omitempty"`
}

// Conversation is an ordered list of messages.
type Conversation struct {
	Messages []Message `json:"messages"`
}

// FromMessages overwrites the conversation with the given messages.
func (c *Conversation) FromMessages(msgs []Message) {
	c.Messages = append([]Message{}, msgs...)
}

// RenderConversationConfig controls rendering behavior (e.g., analysis dropping).
type RenderConversationConfig struct {
	AutoDropAnalysis bool `json:"auto_drop_analysis"`
}

// MarshalJSON implements the JSON shape used by the Harmony format, where
// content may be a string or a list of structured items.
func (m *Message) MarshalJSON() ([]byte, error) {
	type raw struct {
		Role        Role   `json:"role"`
		Name        string `json:"name,omitempty"`
		Recipient   string `json:"recipient,omitempty"`
		Content     any    `json:"content"`
		Channel     string `json:"channel,omitempty"`
		ContentType string `json:"content_type,omitempty"`
	}
	r := raw{
		Role:        m.Author.Role,
		Name:        m.Author.Name,
		Recipient:   m.Recipient,
		Channel:     m.Channel,
		ContentType: m.ContentType,
	}
	if len(m.Content) == 1 && m.Content[0].Type == ContentText {
		r.Content = m.Content[0].Text
	} else {
		r.Content = m.Content
	}
	return json.Marshal(r)
}

// UnmarshalJSON accepts either a string or a list for content and populates
// the message accordingly.
func (m *Message) UnmarshalJSON(b []byte) error {
	type raw struct {
		Role        Role            `json:"role"`
		Name        string          `json:"name,omitempty"`
		Recipient   string          `json:"recipient,omitempty"`
		Content     json.RawMessage `json:"content"`
		Channel     string          `json:"channel,omitempty"`
		ContentType string          `json:"content_type,omitempty"`
	}
	var r raw
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}
	m.Author = Author{Role: r.Role, Name: r.Name}
	m.Recipient = r.Recipient
	m.Channel = r.Channel
	m.ContentType = r.ContentType
	// content can be a string or []Content
	var s string
	if err := json.Unmarshal(r.Content, &s); err == nil {
		m.Content = []Content{{Type: ContentText, Text: s}}
		return nil
	}
	var arr []Content
	if err := json.Unmarshal(r.Content, &arr); err != nil {
		return err
	}
	m.Content = arr
	return nil
}

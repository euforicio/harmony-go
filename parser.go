package harmony

import (
	"encoding/json"
	"errors"

	"github.com/euforicio/harmony-go/tokenizer"
)

type streamState int

const (
	stExpectStart streamState = iota
	stHeader
	stContent
)

type parsedHeader struct {
	author      Author
	recipient   string
	channel     string
	contentType string
}

// StreamParser incrementally parses Harmony tokens into messages. It mirrors
// the behavior of the upstream StreamableParser and is useful for streaming.
type StreamParser struct {
	enc         *Encoding
	nextRole    *Role
	state       streamState
	tokens      []uint32
	messages    []Message
	headerToks  []uint32
	contentToks []uint32
	// last delta stored as bytes to avoid per-token string allocs
	lastDeltaBytes []byte
	// scratch buffer reused for per-token decoding to reduce allocations
	scratch []byte
}

// NewStreamParser creates a streaming parser. If role is provided, it is used
// as a hint for the upcoming header and the parser starts in Header state.
func NewStreamParser(enc *Encoding, role *Role) (*StreamParser, error) {
	st := stExpectStart
	if role != nil {
		// Match upstream behaviour: if a next role is hinted, begin collecting header tokens
		// immediately until we see <|message|>.
		st = stHeader
	}
	return &StreamParser{enc: enc, nextRole: role, state: st}, nil
}

// Process consumes a single token and updates the parser state.
func (p *StreamParser) Process(token uint32) error {
	p.tokens = append(p.tokens, token)
	switch p.state {
	case stExpectStart:
		if token == tokenizer.TokStart {
			p.headerToks = p.headerToks[:0]
			p.state = stHeader
			return nil
		}
		return errors.New("unexpected token while expecting <|start|>")
	case stHeader:
		if token == tokenizer.TokStart {
			// Ignore stray start tokens when beginning in Header due to role hint
			return nil
		}
		if token == tokenizer.TokMessage {
			// parse header tokens
			hdr, err := p.parseHeaderFromTokens(p.headerToks)
			if err != nil {
				return err
			}
			// set state
			p.nextRole = nil
			p.contentToks = p.contentToks[:0]
			// store header in next message via zero-width marker: we carry as separate field? we'll stash in struct
			// Encapsulate header in a new message placeholder using content later
			p.messages = append(p.messages, Message{Author: hdr.author, Recipient: hdr.recipient, Channel: hdr.channel, ContentType: hdr.contentType})
			p.state = stContent
			return nil
		}
		p.headerToks = append(p.headerToks, token)
		return nil
	case stContent:
		// stop tokens finalize message
		if _, stop := p.enc.stopAll[token]; stop {
			if err := p.finalizeMessage(); err != nil {
				return err
			}
			p.state = stExpectStart
			return nil
		}
		// Append token to logical content
		p.contentToks = append(p.contentToks, token)
		// Decode only this token into scratch and set delta to the decoded bytes
		p.scratch = p.scratch[:0]
		one := [...]uint32{token}
		if err := p.enc.bpe.DecodeBytesInto(&p.scratch, one[:]); err != nil {
			return err
		}
		// Save bytes; conversion to string is deferred to LastContentDelta.
		p.lastDeltaBytes = append(p.lastDeltaBytes[:0], p.scratch...)
		return nil
	default:
		return errors.New("invalid parser state")
	}
}

func (p *StreamParser) finalizeMessage() error {
	if len(p.messages) == 0 {
		return nil
	}
	idx := len(p.messages) - 1
	text, err := p.enc.bpe.DecodeUTF8(p.contentToks)
	if err != nil {
		return err
	}
	p.messages[idx].Content = []Content{{Type: ContentText, Text: text}}
	// reset buffers
	p.headerToks = p.headerToks[:0]
	p.contentToks = p.contentToks[:0]
	return nil
}

// ProcessEOS flushes any buffered content and finalizes the current message.
func (p *StreamParser) ProcessEOS() error {
	if p.state == stContent {
		return p.finalizeMessage()
	}
	return nil
}

// Messages returns all fully parsed messages so far.
func (p *StreamParser) Messages() []Message { return append([]Message(nil), p.messages...) }

// Tokens returns all tokens that have been fed to the parser.
func (p *StreamParser) Tokens() []uint32 { return append([]uint32(nil), p.tokens...) }

// StateJSON exposes the current state for interop/debugging.
func (p *StreamParser) StateJSON() (string, error) {
	state := struct {
		State string `json:"state"`
	}{State: map[streamState]string{stExpectStart: "ExpectStart", stHeader: "Header", stContent: "Content"}[p.state]}
	b, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CurrentRole returns the role of the current message if known, otherwise the
// next role hint. Nil indicates the role is not yet known.
func (p *StreamParser) CurrentRole() *Role {
	switch p.state {
	case stContent:
		if len(p.messages) == 0 {
			return nil
		}
		r := p.messages[len(p.messages)-1].Author.Role
		return &r
	default:
		return p.nextRole
	}
}

// CurrentContent returns the textual content accumulated so far for the
// current message. Returns an empty string if no content is in progress.
func (p *StreamParser) CurrentContent() string {
	if p.state != stContent {
		return ""
	}
	s, err := p.enc.bpe.DecodeUTF8(p.contentToks)
	if err != nil {
		return ""
	}
	return s
}

// CurrentContentType returns the content-type marker (e.g., "<|constrain|>json")
// for the current message if known.
func (p *StreamParser) CurrentContentType() string {
	if p.state != stContent || len(p.messages) == 0 {
		return ""
	}
	return p.messages[len(p.messages)-1].ContentType
}

// CurrentChannel returns the channel for the current message if known.
func (p *StreamParser) CurrentChannel() string {
	if p.state != stContent || len(p.messages) == 0 {
		return ""
	}
	return p.messages[len(p.messages)-1].Channel
}

// CurrentRecipient returns the recipient for the current message if known.
func (p *StreamParser) CurrentRecipient() string {
	if p.state != stContent || len(p.messages) == 0 {
		return ""
	}
	return p.messages[len(p.messages)-1].Recipient
}

// LastContentDelta returns the most recent decoded fragment since the last
// Process call, if any.
func (p *StreamParser) LastContentDelta() string { return string(p.lastDeltaBytes) }

func (p *StreamParser) parseHeaderFromTokens(header []uint32) (parsedHeader, error) {
	var hdr parsedHeader
	// decode utf8
	s, err := p.enc.bpe.DecodeUTF8(header)
	if err != nil {
		return hdr, err
	}
	s = normalizeHeader(s)
	roleToken, remainder := splitLeadingToken(s)

	detectedRole, nameFromHeader := detectRoleAndAuthor(roleToken, remainder)

	hdr.author.Role = detectedRole
	hdr.author.Name = nameFromHeader
	if p.nextRole != nil {
		hdr.author.Role = *p.nextRole
		if hdr.author.Role == RoleTool && hdr.author.Name == "" {
			hdr.author.Name = nameFromHeader
		}
	}
	// channel
	hdr.channel = extractChannel(s)
	// recipient
	hdr.recipient = extractRecipient(s)
	// content type: remove known parts and trim
	if ct := scrubContentType(roleToken, remainder); ct != "" {
		hdr.contentType = ct
	}
	return hdr, nil
}

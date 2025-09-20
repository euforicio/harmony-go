package harmony

import "strings"

// normalizeHeader inserts spaces before meta markers that may appear adjacent
// to tokens so that simple whitespace splitting is reliable.
func normalizeHeader(s string) string {
	if strings.Contains(s, "<|channel|>") {
		s = strings.TrimSpace(strings.ReplaceAll(s, "<|channel|>", " <|channel|>"))
	}
	if strings.Contains(s, "<|constrain|>") {
		s = strings.TrimSpace(strings.ReplaceAll(s, "<|constrain|>", " <|constrain|>"))
	}
	return s
}

// splitLeadingToken returns the first token up to a space or '<', and the
// remainder trimmed of leading whitespace.
func splitLeadingToken(s string) (string, string) {
	stop := len(s)
	for i, ch := range s {
		if ch == ' ' || ch == '<' {
			stop = i
			break
		}
	}
	roleToken := s[:stop]
	rem := ""
	if stop < len(s) {
		rem = strings.TrimSpace(s[stop:])
	}
	return roleToken, rem
}

// nextValueToken returns the first token in input that isn't a meta token
// like to=... or a special marker starting with <|.
func nextValueToken(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	end := len(input)
	for i, ch := range input {
		if ch == ' ' || ch == '<' {
			end = i
			break
		}
	}
	token := input[:end]
	if strings.HasPrefix(token, "to=") || strings.HasPrefix(token, "<|") {
		return ""
	}
	return token
}

// detectRoleAndAuthor infers the role from the header's leading token and
// recovers the author name when applicable (especially for tools).
func detectRoleAndAuthor(roleToken, remainder string) (Role, string) {
	var detected Role
	switch {
	case roleToken == string(RoleUser) || strings.HasPrefix(roleToken, string(RoleUser)+":"):
		detected = RoleUser
	case roleToken == string(RoleAssistant) || strings.HasPrefix(roleToken, string(RoleAssistant)+":"):
		detected = RoleAssistant
	case roleToken == string(RoleSystem) || strings.HasPrefix(roleToken, string(RoleSystem)+":"):
		detected = RoleSystem
	case roleToken == string(RoleDeveloper) || strings.HasPrefix(roleToken, string(RoleDeveloper)+":"):
		detected = RoleDeveloper
	default:
		detected = RoleTool
	}

	// author name (for tools or role:name aliases)
	name := ""
	if detected == RoleTool {
		switch {
		case strings.HasPrefix(roleToken, "tool:"):
			name = roleToken[len("tool:"):]
		case roleToken == string(RoleTool):
			name = nextValueToken(remainder)
		case roleToken != "":
			name = roleToken
		}
		if name == "" {
			name = nextValueToken(remainder)
		}
	} else {
		aliasPrefix := string(detected) + ":"
		if strings.HasPrefix(roleToken, aliasPrefix) {
			name = roleToken[len(aliasPrefix):]
		}
	}

	return detected, name
}

func extractChannel(s string) string {
	if idx := strings.Index(s, "<|channel|>"); idx != -1 {
		after := s[idx+len("<|channel|>"):]
		end := strings.IndexByte(after, ' ')
		if end == -1 {
			return after
		}
		return after[:end]
	}
	return ""
}

func extractRecipient(s string) string {
	if idx := strings.Index(s, " to="); idx != -1 {
		after := s[idx+len(" to="):]
		end := -1
		for i := 0; i < len(after); i++ {
			if after[i] == ' ' || after[i] == '<' {
				end = i
				break
			}
		}
		if end == -1 {
			return after
		}
		return after[:end]
	}
	return ""
}

// scrubContentType computes the trailing content type marker (e.g. <|constrain|>json)
// by starting from the header remainder and removing role/alias prefixes, recipient,
// and any channel annotations.
func scrubContentType(roleToken, remainder string) string {
	ss := remainder
	// strip role/name prefix
	for _, r := range []string{"assistant", "user", "system", "developer"} {
		if strings.HasPrefix(ss, r) {
			ss = ss[len(r):]
			if strings.HasPrefix(ss, ":") {
				ss = ss[1:]
				sp := strings.IndexByte(ss, ' ')
				if sp >= 0 {
					ss = ss[sp:]
				} else {
					ss = ""
				}
			}
			break
		}
	}
	// strip recipient
	if strings.HasPrefix(ss, "to=") {
		after := ss[len("to="):]
		sp := strings.IndexByte(after, ' ')
		if sp == -1 {
			ss = ""
		} else {
			ss = strings.TrimSpace(after[sp:])
		}
	} else if idx := strings.Index(ss, " to="); idx != -1 {
		before := ss[:idx]
		after := ss[idx+len(" to="):]
		sp := strings.IndexByte(after, ' ')
		if sp == -1 {
			ss = strings.TrimSpace(before)
		} else {
			ss = strings.TrimSpace(before + after[sp:])
		}
	}
	// strip channel sequences
	for {
		idx := strings.Index(ss, "<|channel|>")
		if idx == -1 {
			break
		}
		after := ss[idx+len("<|channel|>"):]
		sp := strings.IndexByte(after, ' ')
		if sp == -1 {
			ss = strings.TrimSpace(ss[:idx])
		} else {
			ss = strings.TrimSpace(ss[:idx] + after[sp:])
		}
	}
	ss = strings.TrimSpace(ss)
	return ss
}

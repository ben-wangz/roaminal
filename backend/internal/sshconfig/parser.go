package sshconfig

import (
	"encoding/base64"
	"strconv"
	"strings"
	"unicode"

	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

func Parse(data []byte, capability sshfs.Capability) Document {
	doc := Document{Bytes: append([]byte(nil), data...), Newline: "\n", Capability: capability}
	if strings.Contains(string(data), "\r\n") {
		doc.Newline = "\r\n"
	}
	current := Block{Kind: Global, Start: 0}
	for offset, lineNo := 0, 1; offset < len(data) || (offset == 0 && len(data) == 0); lineNo++ {
		end := strings.IndexByte(string(data[offset:]), '\n')
		lineEnd := len(data)
		if end >= 0 {
			lineEnd = offset + end + 1
		}
		contentEnd := lineEnd
		if contentEnd > offset && data[contentEnd-1] == '\n' {
			contentEnd--
		}
		if contentEnd > offset && data[contentEnd-1] == '\r' {
			contentEnd--
		}
		raw := data[offset:lineEnd]
		if directive, ok := scanDirective(string(data[offset:contentEnd]), offset, lineNo); ok {
			key := strings.ToLower(directive.Keyword)
			if key == "host" || key == "match" {
				current.End = offset
				if current.Kind != Global {
					doc.Blocks = append(doc.Blocks, current)
				}
				kind := HostBlock
				if key == "match" {
					kind = MatchBlock
				}
				current = Block{Kind: kind, Start: offset, Header: directive}
				if kind == HostBlock && len(directive.Tokens) == 1 && concreteAlias(directive.Tokens[0]) {
					current.Alias = directive.Tokens[0]
				}
			} else {
				if current.Kind == Global || current.Kind == HostBlock || current.Kind == MatchBlock {
					current.Directives = append(current.Directives, directive)
				}
				if key == "include" || !supported(key) {
					doc.Warnings = append(doc.Warnings, Warning{Directive: directive.Keyword, Line: lineNo, Class: warningClass(key)})
				}
			}
		}
		_ = raw
		if end < 0 {
			offset = len(data)
			break
		}
		offset = lineEnd
	}
	current.End = len(data)
	if current.Kind != Global {
		doc.Blocks = append(doc.Blocks, current)
	}
	return doc
}

func scanDirective(line string, offset, lineNo int) (Directive, bool) {
	trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return Directive{}, false
	}
	indent := len(line) - len(trimmed)
	keyEnd := 0
	for keyEnd < len(trimmed) && !unicode.IsSpace(rune(trimmed[keyEnd])) && trimmed[keyEnd] != '=' {
		keyEnd++
	}
	if keyEnd == 0 {
		return Directive{}, false
	}
	keyword := trimmed[:keyEnd]
	valueStart := keyEnd
	for valueStart < len(trimmed) && (unicode.IsSpace(rune(trimmed[valueStart])) || trimmed[valueStart] == '=') {
		valueStart++
	}
	value := strings.TrimSpace(trimmed[valueStart:])
	tokens := tokenize(value)
	absoluteValue := offset + indent + valueStart
	valueEnd := absoluteValue + len(value)
	return Directive{Keyword: keyword, Value: value, Tokens: tokens, Line: lineNo, LineStart: offset, LineEnd: offset + len(line), ValueStart: absoluteValue, ValueEnd: valueEnd, Raw: []byte(line)}, true
}

func tokenize(value string) []string {
	var result []string
	var current strings.Builder
	quote := rune(0)
	escaped := false
	for _, r := range value {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if unicode.IsSpace(r) {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
			continue
		}
		if r == '#' && current.Len() == 0 {
			break
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result
}

func supported(key string) bool {
	switch key {
	case "hostname", "user", "port", "identityfile", "identitiesonly", "stricthostkeychecking", "userknownhostsfile", "serveraliveinterval":
		return true
	}
	return false
}
func warningClass(key string) string {
	if key == "include" {
		return "include_ignored"
	}
	return "advanced_directive"
}
func concreteAlias(alias string) bool {
	return alias != "" && !strings.ContainsAny(alias, "*!?") && !strings.HasPrefix(alias, "!")
}

func DefinitionID(alias string) string {
	return "ssh." + base64.RawURLEncoding.EncodeToString([]byte(alias))
}

func (d Document) Definitions(knownKeys map[string]bool) []Definition {
	result := make([]Definition, 0)
	for _, block := range d.Blocks {
		if block.Kind != HostBlock || block.Alias == "" {
			continue
		}
		definition := definitionFromBlock(block, knownKeys, d.Warnings)
		result = append(result, definition)
	}
	return result
}

func definitionFromBlock(block Block, knownKeys map[string]bool, warnings []Warning) Definition {
	result := Definition{ConnectionDefinitionID: DefinitionID(block.Alias), Type: "ssh", HostAlias: block.Alias, IdentityFileNames: []string{}, Warnings: append([]Warning(nil), warnings...), Capabilities: map[string]bool{"edit": true, "delete": true}}
	trustUnknown := false
	for _, directive := range block.Directives {
		key := strings.ToLower(directive.Keyword)
		if len(directive.Tokens) == 0 {
			continue
		}
		value := directive.Tokens[0]
		switch key {
		case "hostname":
			result.HostName = stringPtr(value)
		case "user":
			result.User = stringPtr(value)
		case "port":
			if n, err := strconv.ParseUint(value, 10, 16); err == nil && n > 0 {
				v := uint16(n)
				result.Port = &v
			}
		case "identityfile":
			if name, ok := managedIdentity(value); ok && knownKeys[name] {
				result.IdentityFileNames = append(result.IdentityFileNames, name)
			} else {
				result.UnmanagedIdentityCount++
			}
		case "identitiesonly":
			v := strings.ToLower(value)
			if v == "yes" || v == "no" {
				result.IdentitiesOnly = &v
			}
		case "stricthostkeychecking":
			v := strings.ToLower(value)
			if v == "no" {
				result.StrictHostKeyChecking = &v
				result.HostVerificationAssessment = "weakened"
			} else {
				trustUnknown = true
			}
		case "userknownhostsfile":
			if value == "/dev/null" {
				result.UserKnownHostsFile = stringPtr(value)
				result.HostVerificationAssessment = "weakened"
			} else {
				trustUnknown = true
			}
		case "serveraliveinterval":
			if n, err := strconv.ParseUint(value, 10, 32); err == nil {
				v := uint32(n)
				result.ServerAliveInterval = &v
			}
		default:
			result.AdvancedDirectiveCount++
		}
	}
	if trustUnknown || result.HostVerificationAssessment == "" {
		result.HostVerificationAssessment = "default"
		if trustUnknown {
			result.HostVerificationAssessment = "unknown"
		}
	}
	return result
}

func managedIdentity(value string) (string, bool) {
	value = strings.TrimPrefix(value, "~/.ssh/")
	if strings.Contains(value, "/") || value == "" {
		return "", false
	}
	if value == "id_ed25519" || value == "id_rsa" || strings.HasSuffix(value, "_ed25519") || strings.HasSuffix(value, "_rsa") {
		return value, true
	}
	return "", false
}
func stringPtr(value string) *string { return &value }

package sshconfig

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type replacement struct {
	start, end int
	data       []byte
}

func patchBlock(doc Document, alias string, edit Edit) ([]byte, error) {
	block := findBlock(doc, alias)
	if block == nil {
		return nil, os.ErrNotExist
	}
	newAlias := edit.HostAlias
	if newAlias == "" {
		newAlias = alias
	}
	if !validAlias(newAlias) {
		return nil, errors.New("invalid host alias")
	}
	if newAlias != alias && findBlock(doc, newAlias) != nil {
		return nil, errors.New("host alias already exists")
	}
	desired := map[string]*string{}
	if edit.HostName != nil {
		desired["hostname"] = edit.HostName
	}
	if edit.User != nil {
		desired["user"] = edit.User
	}
	if edit.Port != nil {
		value := strconv.FormatUint(uint64(*edit.Port), 10)
		desired["port"] = &value
	}
	if edit.IdentitiesOnly != nil {
		desired["identitiesonly"] = edit.IdentitiesOnly
	}
	if edit.StrictHostKeyChecking != nil {
		desired["stricthostkeychecking"] = edit.StrictHostKeyChecking
	}
	if edit.UserKnownHostsFile != nil {
		desired["userknownhostsfile"] = edit.UserKnownHostsFile
	}
	if edit.ServerAliveInterval != nil {
		value := strconv.FormatUint(uint64(*edit.ServerAliveInterval), 10)
		desired["serveraliveinterval"] = &value
	}
	counts := map[string]int{}
	for _, directive := range block.Directives {
		key := strings.ToLower(directive.Keyword)
		if supported(key) {
			counts[key]++
		}
	}
	for key, count := range counts {
		if key != "identityfile" && count > 1 {
			return nil, fmt.Errorf("%w: %s", ErrFieldNotEditable, key)
		}
	}
	replacements := make([]replacement, 0)
	seen := map[string]bool{}
	identityRequested := edit.IdentityFileNames != nil
	identityInserted := false
	for _, directive := range block.Directives {
		key := strings.ToLower(directive.Keyword)
		if key == "host" || key == "match" {
			continue
		}
		if directive.LineStart < block.Start || directive.LineStart >= block.End {
			continue
		}
		if key == "identityfile" && managedIdentityToken(directive) {
			if !identityRequested {
				continue
			}
			if !identityInserted {
				replacements = append(replacements, replacement{directive.LineStart, directive.LineEnd, []byte(renderIdentityLines(edit.IdentityFileNames, doc.Newline))})
				identityInserted = true
			} else {
				replacements = append(replacements, replacement{directive.LineStart, directive.LineEnd, nil})
			}
			continue
		}
		value, ok := desired[key]
		if !ok {
			continue
		}
		seen[key] = true
		if value == nil {
			replacements = append(replacements, replacement{directive.LineStart, directive.LineEnd, nil})
			continue
		}
		if len(directive.Tokens) == 0 {
			continue
		}
		end := directive.ValueStart + len(directive.Tokens[0])
		replacements = append(replacements, replacement{directive.ValueStart, end, []byte(*value)})
	}
	if identityRequested && !identityInserted && len(edit.IdentityFileNames) > 0 {
		replacements = append(replacements, replacement{block.End, block.End, []byte(renderIdentityLines(edit.IdentityFileNames, doc.Newline))})
	}
	for key, value := range desired {
		if value != nil && !seen[key] {
			replacements = append(replacements, replacement{block.End, block.End, []byte(renderDirective(key, *value, doc.Newline))})
		}
	}
	if newAlias != alias {
		d := block.Header
		end := d.ValueStart + len(d.Tokens[0])
		replacements = append(replacements, replacement{d.ValueStart, end, []byte(newAlias)})
	}
	return applyReplacements(doc.Bytes, replacements), nil
}

func appendBlock(doc Document, edit Edit) []byte {
	newline := doc.Newline
	if newline == "" {
		newline = "\n"
	}
	var b strings.Builder
	if len(doc.Bytes) > 0 && !strings.HasSuffix(string(doc.Bytes), newline) {
		b.WriteString(newline)
	}
	b.WriteString("Host ")
	b.WriteString(edit.HostAlias)
	b.WriteString(newline)
	if edit.HostName != nil {
		b.WriteString(renderDirective("hostname", *edit.HostName, newline))
	}
	if edit.User != nil {
		b.WriteString(renderDirective("user", *edit.User, newline))
	}
	if edit.Port != nil {
		b.WriteString(renderDirective("port", strconv.FormatUint(uint64(*edit.Port), 10), newline))
	}
	if edit.IdentityFileNames != nil {
		b.WriteString(renderIdentityLines(edit.IdentityFileNames, newline))
	}
	if edit.IdentitiesOnly != nil {
		b.WriteString(renderDirective("identitiesonly", *edit.IdentitiesOnly, newline))
	}
	if edit.StrictHostKeyChecking != nil {
		b.WriteString(renderDirective("stricthostkeychecking", *edit.StrictHostKeyChecking, newline))
	}
	if edit.UserKnownHostsFile != nil {
		b.WriteString(renderDirective("userknownhostsfile", *edit.UserKnownHostsFile, newline))
	}
	if edit.ServerAliveInterval != nil {
		b.WriteString(renderDirective("serveraliveinterval", strconv.FormatUint(uint64(*edit.ServerAliveInterval), 10), newline))
	}
	return append(append([]byte{}, doc.Bytes...), []byte(b.String())...)
}

func renderDirective(key, value, newline string) string {
	return "  " + canonicalKey(key) + " " + value + newline
}
func renderIdentityLines(values []string, newline string) string {
	var b strings.Builder
	for _, value := range values {
		b.WriteString(renderDirective("identityfile", "~/.ssh/"+value, newline))
	}
	return b.String()
}
func canonicalKey(key string) string {
	switch key {
	case "hostname":
		return "HostName"
	case "identitiesonly":
		return "IdentitiesOnly"
	case "identityfile":
		return "IdentityFile"
	case "stricthostkeychecking":
		return "StrictHostKeyChecking"
	case "userknownhostsfile":
		return "UserKnownHostsFile"
	case "serveraliveinterval":
		return "ServerAliveInterval"
	}
	return strings.Title(key)
}

func applyReplacements(data []byte, replacements []replacement) []byte {
	for i := 1; i < len(replacements); i++ {
		for j := i; j > 0 && replacements[j].start > replacements[j-1].start; j-- {
			replacements[j], replacements[j-1] = replacements[j-1], replacements[j]
		}
	}
	result := append([]byte{}, data...)
	for _, item := range replacements {
		result = append(append(append([]byte{}, result[:item.start]...), item.data...), result[item.end:]...)
	}
	return result
}

func findBlock(doc Document, alias string) *Block {
	for index := range doc.Blocks {
		if doc.Blocks[index].Kind == HostBlock && doc.Blocks[index].Alias == alias {
			return &doc.Blocks[index]
		}
	}
	return nil
}
func validAlias(alias string) bool {
	if len(alias) < 1 || len(alias) > 255 {
		return false
	}
	for index, r := range alias {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') || index == 0 && (r < 'A' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}
func managedIdentityToken(d Directive) bool {
	if len(d.Tokens) == 0 {
		return false
	}
	_, ok := managedIdentity(d.Tokens[0])
	return ok
}

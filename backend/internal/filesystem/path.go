package filesystem

import (
	"path"
	"strings"
	"unicode/utf8"
)

func ValidateRelativePath(value string) (string, error) {
	if value == "" || value == "." {
		return ".", nil
	}
	if !utf8.ValidString(value) || strings.HasPrefix(value, "/") {
		return "", ErrInvalidPath
	}
	if strings.ContainsRune(value, '\\') || strings.ContainsRune(value, 0) || strings.ContainsRune(value, '\r') || strings.ContainsRune(value, '\n') {
		return "", ErrInvalidPath
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return "", ErrPathOutsideRoot
		}
	}
	clean := path.Clean(value)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", ErrPathOutsideRoot
	}
	if clean == "" || clean == "." {
		return ".", nil
	}
	if strings.HasPrefix(clean, "/") {
		return "", ErrInvalidPath
	}
	return clean, nil
}

func JoinRelative(parent, name string) string {
	if parent == "." {
		return name
	}
	return path.Join(parent, name)
}

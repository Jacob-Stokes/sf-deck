// Package redact removes credentials from diagnostic and user-facing output.
package redact

import (
	"bytes"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const replacement = "<redacted>"

var (
	mu      sync.RWMutex
	secrets []string

	patterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bBearer[ \t]+[A-Za-z0-9._~+/=-]+`),
		regexp.MustCompile(`\b00D[A-Za-z0-9]{12}(?:[A-Za-z0-9]{3})?![A-Za-z0-9._~+/=-]{20,}`),
		regexp.MustCompile(`(?i)\bforce://[^\s"'<>]+`),
		regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`),
		regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}`),
		regexp.MustCompile(`(?s)-----BEGIN(?: [A-Z0-9]+)? PRIVATE KEY-----.*?-----END(?: [A-Z0-9]+)? PRIVATE KEY-----`),
	}
)

// RegisterSecret adds a runtime credential to the exact-match redaction set.
// The URL-encoded form is registered too because tokens may appear in URLs.
func RegisterSecret(secret string) {
	secret = strings.TrimSpace(secret)
	if secret == "" || secret == replacement {
		return
	}
	variants := []string{secret}
	if escaped := url.QueryEscape(secret); escaped != secret {
		variants = append(variants, escaped)
	}
	if quoted := strconv.Quote(secret); len(quoted) >= 2 {
		escaped := quoted[1 : len(quoted)-1]
		if escaped != secret {
			variants = append(variants, escaped)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	for _, candidate := range variants {
		found := false
		for _, existing := range secrets {
			if existing == candidate {
				found = true
				break
			}
		}
		if !found {
			secrets = append(secrets, candidate)
		}
	}
	// Replace longer values first if one registered secret contains another.
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
}

// String redacts known secret shapes and every registered runtime credential.
func String(value string) string {
	return string(Bytes([]byte(value)))
}

// Bytes is the binary-safe form of String. Unmatched bytes are preserved.
func Bytes(value []byte) []byte {
	out := append([]byte(nil), value...)
	mu.RLock()
	registered := append([]string(nil), secrets...)
	mu.RUnlock()
	for _, secret := range registered {
		out = bytes.ReplaceAll(out, []byte(secret), []byte(replacement))
	}
	for _, pattern := range patterns {
		out = pattern.ReplaceAll(out, []byte(replacement))
	}
	return out
}

// Strings returns a redacted copy of values.
func Strings(values []string) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = String(value)
	}
	return out
}

// Map redacts string material recursively in a JSON-like map.
func Map(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[String(key)] = anyValue(value)
	}
	return out
}

func anyValue(value any) any {
	switch typed := value.(type) {
	case string:
		return String(typed)
	case error:
		return String(typed.Error())
	case []string:
		return Strings(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = anyValue(item)
		}
		return out
	case map[string]any:
		return Map(typed)
	default:
		return value
	}
}

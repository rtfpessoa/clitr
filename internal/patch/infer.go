package patch

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode"
)

// InferGoType examines a raw JSON value and returns the appropriate Go type string.
// It uses a conservative mapping: objects, arrays, and nulls all map to json.RawMessage.
func InferGoType(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "json.RawMessage"
	}

	switch trimmed[0] {
	case '"':
		return "string"
	case 't', 'f':
		return "bool"
	case '{', '[', 'n':
		return "json.RawMessage"
	default:
		// Digits or minus sign indicate a number.
		// Go's encoding/json always decodes numbers as float64.
		return "float64"
	}
}

// commonAcronyms maps lowercase acronyms to their Go-idiomatic uppercase forms.
var commonAcronyms = map[string]string{
	"id":   "ID",
	"ids":  "IDs",
	"url":  "URL",
	"uri":  "URI",
	"html": "HTML",
	"css":  "CSS",
	"api":  "API",
	"json": "JSON",
	"xml":  "XML",
	"http": "HTTP",
	"https": "HTTPS",
	"ip":   "IP",
	"sql":  "SQL",
	"ssh":  "SSH",
	"tls":  "TLS",
	"tcp":  "TCP",
	"udp":  "UDP",
	"iban": "IBAN",
	"cta":  "CTA",
}

// JSONKeyToGoFieldName converts a camelCase (or kebab-case) JSON key to a
// PascalCase Go exported field name, respecting common Go acronym conventions.
func JSONKeyToGoFieldName(jsonKey string) string {
	// Split on non-alphanumeric characters (handles kebab-case like "card-dispute-txId")
	parts := splitJSONKey(jsonKey)

	var result strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		if upper, ok := commonAcronyms[strings.ToLower(part)]; ok {
			result.WriteString(upper)
		} else {
			// Capitalize first letter
			runes := []rune(part)
			runes[0] = unicode.ToUpper(runes[0])
			result.WriteString(string(runes))
		}
	}

	return result.String()
}

// splitJSONKey splits a JSON key into its component words.
// It handles camelCase ("firstName" -> ["first", "Name"]) and
// kebab-case ("card-dispute-txId" -> ["card", "dispute", "tx", "Id"]).
func splitJSONKey(key string) []string {
	// First split on non-alphanumeric characters
	segments := splitOnNonAlpha(key)

	// Then split each segment on camelCase boundaries
	var parts []string
	for _, seg := range segments {
		parts = append(parts, splitCamelCase(seg)...)
	}

	return parts
}

// splitOnNonAlpha splits a string on non-alphanumeric characters.
func splitOnNonAlpha(s string) []string {
	var parts []string
	var current strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// splitCamelCase splits a camelCase string into its component words.
// "firstName" -> ["first", "Name"]
// "HTMLContent" -> ["HTML", "Content"]
func splitCamelCase(s string) []string {
	if s == "" {
		return nil
	}

	runes := []rune(s)
	var parts []string
	start := 0

	for i := 1; i < len(runes); i++ {
		// Split before an uppercase letter that follows a lowercase letter
		if unicode.IsUpper(runes[i]) && unicode.IsLower(runes[i-1]) {
			parts = append(parts, string(runes[start:i]))
			start = i
		}
		// Split before the last letter of an uppercase run followed by lowercase
		// e.g., "HTMLContent" -> split before "C": "HTML" + "Content"
		if i > start+1 && unicode.IsUpper(runes[i-1]) && unicode.IsLower(runes[i]) {
			parts = append(parts, string(runes[start:i-1]))
			start = i - 1
		}
	}

	parts = append(parts, string(runes[start:]))
	return parts
}

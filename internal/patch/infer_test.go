package patch

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInferGoType(t *testing.T) {
	tests := []struct {
		name     string
		raw      json.RawMessage
		expected string
	}{
		{"string value", json.RawMessage(`"hello"`), "string"},
		{"empty string", json.RawMessage(`""`), "string"},
		{"bool true", json.RawMessage(`true`), "bool"},
		{"bool false", json.RawMessage(`false`), "bool"},
		{"integer number", json.RawMessage(`42`), "float64"},
		{"float number", json.RawMessage(`3.14`), "float64"},
		{"negative number", json.RawMessage(`-10`), "float64"},
		{"null value", json.RawMessage(`null`), "json.RawMessage"},
		{"object value", json.RawMessage(`{"key": "value"}`), "json.RawMessage"},
		{"empty object", json.RawMessage(`{}`), "json.RawMessage"},
		{"array value", json.RawMessage(`[1, 2, 3]`), "json.RawMessage"},
		{"empty array", json.RawMessage(`[]`), "json.RawMessage"},
		{"whitespace padded string", json.RawMessage(`  "hello"  `), "string"},
		{"whitespace padded number", json.RawMessage(`  42  `), "float64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InferGoType(tt.raw)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestJSONKeyToGoFieldName(t *testing.T) {
	tests := []struct {
		jsonKey  string
		expected string
	}{
		{"name", "Name"},
		{"firstName", "FirstName"},
		{"eventType", "EventType"},
		{"id", "ID"},
		{"htmlContent", "HTMLContent"},
		{"savingsPlanId", "SavingsPlanID"},
		{"iban", "IBAN"},
		{"contextCategory", "ContextCategory"},
		{"cashAccountNumber", "CashAccountNumber"},
		{"cta", "CTA"},
		{"uri", "URI"},
		{"url", "URL"},
		{"customerSupportChatVisible", "CustomerSupportChatVisible"},
		{"card-dispute-txId", "CardDisputeTxID"},
	}

	for _, tt := range tests {
		t.Run(tt.jsonKey, func(t *testing.T) {
			result := JSONKeyToGoFieldName(tt.jsonKey)
			assert.Equal(t, tt.expected, result)
		})
	}
}

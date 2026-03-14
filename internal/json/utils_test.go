package json

import (
	"testing"

	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshal_ValidJSON(t *testing.T) {
	data := []byte(`{"name": "test", "value": 42}`)

	var result struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	err := Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "test", result.Name)
	assert.Equal(t, 42, result.Value)
}

func TestUnmarshal_RejectsUnknownFields(t *testing.T) {
	data := []byte(`{"name": "test", "unknown_field": "value"}`)

	var result struct {
		Name string `json:"name"`
	}

	err := Unmarshal(data, &result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestMarshal_IndentedOutput(t *testing.T) {
	input := struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}{
		Name:  "test",
		Value: 42,
	}

	data, err := Marshal(input)
	require.NoError(t, err)

	snaps.MatchSnapshot(t, string(data))
}

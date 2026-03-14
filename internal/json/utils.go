package json

import (
	"bytes"
	"encoding/json"
	"io"
)

type RawMessage = json.RawMessage

func Unmarshal(data []byte, v any) error {
	return NewDecoder(bytes.NewReader(data)).Decode(v)
}

func NewDecoder(r io.Reader) *json.Decoder {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	return dec
}

func Marshal(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

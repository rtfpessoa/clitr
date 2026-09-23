package types

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// decodeArrayOrObject decodes API fields that vary between a list and an object.
func decodeArrayOrObject[Item, Object any](raw json.RawMessage) ([]Item, *Object, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 {
		return nil, nil, nil
	}
	var items []Item
	var object *Object
	var err error
	switch value[0] {
	case '[':
		err = json.Unmarshal(value, &items)
	case '{':
		object = new(Object)
		err = json.Unmarshal(value, object)
	default:
		err = fmt.Errorf("unexpected data type: %s", string(value))
	}
	return items, object, err
}

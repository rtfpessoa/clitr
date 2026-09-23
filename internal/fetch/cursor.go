package fetch

import (
	"encoding/base64"
	stdjson "encoding/json"
	"fmt"
	"time"
)

// pageCursor holds pagination cursor values from the API response.
type pageCursor struct {
	Before *string
	After  *string
}

// cursorPayload is the decoded structure of a base64-encoded cursor string.
type cursorPayload struct {
	Keyset struct {
		EventId   string    `json:"eventId"`
		Timestamp time.Time `json:"timestamp"`
	} `json:"keyset"`
	PageDirection Direction `json:"pageDirection"`
}

func (c *pageCursor) forDirection(direction Direction) *string {
	switch direction {
	case DirectionAfter:
		return c.After
	case DirectionBefore:
		return c.Before
	default:
		return nil
	}
}

// ChangeCursor decodes a base64-encoded cursor string,
// changes its direction, and re-encodes it.
func ChangeCursor(cursorStr *string, direction Direction) (*string, error) {
	if cursorStr == nil {
		if direction == DirectionBefore {
			return nil, fmt.Errorf("cannot change direction to before from empty cursor")
		}
		return nil, nil
	}

	bytes, err := base64.RawStdEncoding.DecodeString(*cursorStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode cursor: %w", err)
	}

	parsed := &cursorPayload{}
	// Use stdlib json for cursor parsing — no unknown field restriction needed
	err = stdjson.Unmarshal(bytes, parsed)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal cursor: %w", err)
	}

	parsed.PageDirection = direction
	cursorBytes, err := stdjson.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cursor: %w", err)
	}

	newCursor := base64.RawStdEncoding.EncodeToString(cursorBytes)
	return &newCursor, nil
}

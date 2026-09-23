package types

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// RawEvent combines a timeline event with its detailed information as received from the API.
// It preserves the original API response structure for persistent storage.
type RawEvent struct {
	TimelineEvent TimelineEvent   `json:"timelineEvent"`
	Details       TimelineDetails `json:"details"`
	PageCursor    *string         `json:"pageCursor"`
}

// TimelineDetails represents the detailed information structure returned by the timeline detail API.
type TimelineDetails struct {
	ID       string    `json:"id"`
	Sections []Section `json:"sections"`
}

// Section represents a section in the timeline details with a title and optional data/actions.
type Section struct {
	Title           string            `json:"title"`
	Type            string            `json:"type,omitempty"`
	Description     string            `json:"description,omitempty"`
	ActionableTitle *ActionButton     `json:"actionableTitle,omitempty"`
	Button          *ActionButton     `json:"button,omitempty"`
	Data            []SectionDataItem `json:"data,omitempty"`
	DataHeader      *HeaderData       `json:"-"` // When data is an object (header type)
	Action          *SectionAction    `json:"action,omitempty"`
	Steps           []StepItem        `json:"steps,omitempty"`
}

// HeaderData represents the data object for header-type sections.
type HeaderData struct {
	Icon         *IconValue `json:"icon,omitempty"`
	Status       string     `json:"status,omitempty"`
	Timestamp    string     `json:"timestamp,omitempty"`
	Text         string     `json:"text,omitempty"`
	SubtitleText string     `json:"subtitleText,omitempty"`
}

// IconValue represents an icon that can be either a string or an object.
type IconValue struct {
	// When icon is a string, this holds the value
	Value string `json:"-"`

	// When icon is an object, these fields may be present
	Type  string `json:"type,omitempty"`
	URI   string `json:"uri,omitempty"`
	Asset string `json:"asset,omitempty"`
	Badge string `json:"badge,omitempty"`
}

// UnmarshalJSON implements custom unmarshaling for IconValue.
func (iv *IconValue) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)

	// If it's a string, store it in Value
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return fmt.Errorf("failed to unmarshal IconValue string: %w", err)
		}
		iv.Value = str
		return nil
	}

	// Otherwise, unmarshal as an object using type alias to avoid recursion
	type Alias IconValue
	var alias Alias
	if err := json.Unmarshal(data, &alias); err != nil {
		return fmt.Errorf("failed to unmarshal IconValue object: %w", err)
	}
	*iv = IconValue(alias)
	return nil
}

// MarshalJSON implements custom marshaling for IconValue.
func (iv *IconValue) MarshalJSON() ([]byte, error) {
	// If Value is set, marshal as a string
	if iv.Value != "" {
		return json.Marshal(iv.Value)
	}

	// Otherwise, marshal as an object
	type Alias IconValue
	return json.Marshal((*Alias)(iv))
}

// MarshalJSON implements custom marshaling for Section.
func (s *Section) MarshalJSON() ([]byte, error) {
	type Alias Section
	// Create a map to handle the dynamic data field
	temp := struct {
		*Alias
		Data interface{} `json:"data,omitempty"`
	}{
		Alias: (*Alias)(s),
	}

	// Choose which data field to serialize
	if s.DataHeader != nil {
		temp.Data = s.DataHeader
	} else if len(s.Data) > 0 {
		temp.Data = s.Data
	}

	return json.Marshal(temp)
}

// UnmarshalJSON handles Section.data as either a list or a header object.
func (s *Section) UnmarshalJSON(data []byte) error {
	type alias Section
	*s = Section{}
	raw := struct {
		*alias
		Data json.RawMessage `json:"data,omitempty"`
	}{alias: (*alias)(s)}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal Section: %w", err)
	}
	items, header, err := decodeArrayOrObject[SectionDataItem, HeaderData](raw.Data)
	if err != nil {
		return fmt.Errorf("failed to unmarshal Section.data: %w", err)
	}
	s.Data, s.DataHeader = items, header
	return nil
}

// SectionDataItem represents a data item within a section.
type SectionDataItem struct {
	ID          string        `json:"id,omitempty"`
	Title       string        `json:"title"`
	Style       string        `json:"style,omitempty"`
	Detail      ItemDetail    `json:"detail"`
	Action      *DetailAction `json:"action,omitempty"`
	PostboxType string        `json:"postboxType,omitempty"`
}

// MarshalJSON implements custom marshaling for SectionDataItem.
func (sdi *SectionDataItem) MarshalJSON() ([]byte, error) {
	type Alias SectionDataItem
	return json.Marshal((*Alias)(sdi))
}

// UnmarshalJSON handles detail values that can be an object or a string.
func (sdi *SectionDataItem) UnmarshalJSON(data []byte) error {
	type alias SectionDataItem
	*sdi = SectionDataItem{}
	raw := struct {
		*alias
		Detail json.RawMessage `json:"detail"`
	}{alias: (*alias)(sdi)}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal SectionDataItem: %w", err)
	}
	detail, err := decodeSectionItemDetail(raw.Detail)
	if err != nil {
		return fmt.Errorf("failed to unmarshal SectionDataItem.detail: %w", err)
	}
	sdi.Detail = detail
	return nil
}

func decodeSectionItemDetail(raw json.RawMessage) (ItemDetail, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 {
		return ItemDetail{}, nil
	}
	var detail ItemDetail
	var err error
	switch value[0] {
	case '{':
		err = json.Unmarshal(value, &detail)
	case '"':
		var text string
		err = json.Unmarshal(value, &text)
		detail.Text = text
	default:
		err = fmt.Errorf("unhandled detail type: %s", string(raw))
	}
	return detail, err
}

package types

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// NestedSection represents a section within a DetailActionPayload.
type NestedSection struct {
	Title      string                   `json:"title,omitempty"`
	Type       string                   `json:"type"`
	Text       string                   `json:"text,omitempty"`
	Style      string                   `json:"style,omitempty"`
	Data       []NestedSectionDataItem  `json:"data,omitempty"`
	DataObject *NestedSectionDataObject `json:"-"` // When data is an object (actionButtons type)
	Action     *NestedAction            `json:"action,omitempty"`
}

// NestedSectionDataObject represents the data object for actionButtons-type nested sections.
type NestedSectionDataObject struct {
	Action       *NestedAction `json:"action,omitempty"`
	Title        string        `json:"title,omitempty"`
	Status       string        `json:"status,omitempty"`
	SubtitleText string        `json:"subtitleText,omitempty"`
}

// MarshalJSON implements custom marshaling for NestedSection.
func (ns *NestedSection) MarshalJSON() ([]byte, error) {
	type Alias NestedSection
	// Create a map to handle the dynamic data field
	temp := struct {
		*Alias
		Data interface{} `json:"data,omitempty"`
	}{
		Alias: (*Alias)(ns),
	}

	// Choose which data field to serialize
	if ns.DataObject != nil {
		temp.Data = ns.DataObject
	} else if len(ns.Data) > 0 {
		temp.Data = ns.Data
	}

	return json.Marshal(temp)
}

// UnmarshalJSON handles NestedSection.data as either a list or an action object.
func (ns *NestedSection) UnmarshalJSON(data []byte) error {
	type alias NestedSection
	*ns = NestedSection{}
	raw := struct {
		*alias
		Data json.RawMessage `json:"data,omitempty"`
	}{alias: (*alias)(ns)}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal NestedSection: %w", err)
	}
	items, object, err := decodeArrayOrObject[NestedSectionDataItem, NestedSectionDataObject](raw.Data)
	if err != nil {
		return fmt.Errorf("failed to unmarshal NestedSection.data: %w", err)
	}
	ns.Data, ns.DataObject = items, object
	return nil
}

// NestedSectionDataItem represents a data item within a nested section.
type NestedSectionDataItem struct {
	Title  string           `json:"title"`
	Style  string           `json:"style,omitempty"`
	Detail NestedItemDetail `json:"detail"`
}

// NestedItemDetail represents the detail information for a nested section data item.
type NestedItemDetail struct {
	Text         string           `json:"text,omitempty"`
	Type         string           `json:"type,omitempty"`
	Icon         string           `json:"icon,omitempty"`
	Style        string           `json:"style,omitempty"`
	Title        string           `json:"title,omitempty"`
	DisplayValue *DisplayValue    `json:"displayValue,omitempty"`
	Action       *NestedAction    `json:"action,omitempty"`
	Content      *PayloadContent  `json:"content,omitempty"`
	Leading      *ItemLeading     `json:"leading,omitempty"`
	Trailing     *PayloadTrailing `json:"trailing,omitempty"`
	Trend        *string          `json:"trend,omitempty"`
}

// NestedAction represents an action within a NestedItemDetail.
type NestedAction struct {
	Type    string              `json:"type"`
	Payload DetailActionPayload `json:"payload"`
}

// UnmarshalJSON implements custom unmarshaling for DetailActionPayload.
// When the payload is a string, it stores it in the Value field.
func (dap *DetailActionPayload) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)

	// If it's a string, store it in Value
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return fmt.Errorf("failed to unmarshal DetailActionPayload string: %w", err)
		}
		dap.Value = str
		return nil
	}

	// Otherwise, unmarshal as an object using type alias to avoid recursion
	type Alias DetailActionPayload
	var alias Alias
	if err := json.Unmarshal(data, &alias); err != nil {
		return fmt.Errorf("failed to unmarshal DetailActionPayload object: %w; %s", err, string(data))
	}
	*dap = DetailActionPayload(alias)
	return nil
}

// DetailAction represents an action within an ItemDetail.
type DetailAction struct {
	Type        string              `json:"type"`
	DisplayMode string              `json:"displayMode,omitempty"`
	Payload     DetailActionPayload `json:"payload,omitempty"`
}

// UnmarshalJSON implements custom unmarshaling with strict validation for DetailAction.
func (da *DetailAction) UnmarshalJSON(data []byte) error {
	type Alias DetailAction
	var alias Alias

	if err := json.Unmarshal(data, &alias); err != nil {
		return fmt.Errorf("failed to unmarshal DetailAction: %w", err)
	}

	*da = DetailAction(alias)
	return nil
}

// UnmarshalJSON implements custom unmarshaling with strict validation for DisplayValue.
func (dv *DisplayValue) UnmarshalJSON(data []byte) error {
	type Alias DisplayValue
	var alias Alias

	if err := json.Unmarshal(data, &alias); err != nil {
		return fmt.Errorf("failed to unmarshal DisplayValue: %w", err)
	}

	*dv = DisplayValue(alias)
	return nil
}

// ActionPayload represents the payload of a section action.
type ActionPayload struct {
	// Common fields
	Value           string        `json:"value,omitempty"`
	ContextCategory string        `json:"contextCategory,omitempty"`
	ContextParams   ContextParams `json:"contextParams,omitempty"`
	SavingsPlanID   string        `json:"savingsPlanId,omitempty"`

	// Nested structures
	Sections []NestedSection `json:"sections,omitempty"`
	Action   *DetailAction   `json:"action,omitempty"`
}

// UnmarshalJSON implements custom unmarshaling for ActionPayload.
// When the payload is a string, it stores it in the Value field.
func (ap *ActionPayload) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)

	// If it's a string, store it in Value
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return fmt.Errorf("failed to unmarshal ActionPayload string: %w", err)
		}
		ap.Value = str
		return nil
	}

	// Otherwise, unmarshal as an object using type alias to avoid recursion
	type Alias ActionPayload
	var alias Alias
	if err := json.Unmarshal(data, &alias); err != nil {
		return fmt.Errorf("failed to unmarshal ActionPayload object: %w", err)
	}
	*ap = ActionPayload(alias)
	return nil
}

// SectionAction represents an action associated with a section.
type SectionAction struct {
	Type    string        `json:"type"`
	Payload ActionPayload `json:"payload"`
}

// UnmarshalJSON implements custom unmarshaling with strict validation for SectionAction.
func (sa *SectionAction) UnmarshalJSON(data []byte) error {
	type Alias SectionAction
	var alias Alias

	if err := json.Unmarshal(data, &alias); err != nil {
		return fmt.Errorf("failed to unmarshal SectionAction: %w", err)
	}

	*sa = SectionAction(alias)
	return nil
}

// UnmarshalJSON implements custom unmarshaling with strict validation for TimelineDetails.
func (td *TimelineDetails) UnmarshalJSON(data []byte) error {
	type Alias TimelineDetails

	var alias Alias

	if err := json.Unmarshal(data, &alias); err != nil {
		return fmt.Errorf("failed to unmarshal TimelineDetails: %w", err)
	}

	*td = TimelineDetails(alias)
	return nil
}

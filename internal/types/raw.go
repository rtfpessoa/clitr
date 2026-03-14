package types

import (
	"bytes"
	"fmt"

	"github.com/rtfpessoa/clitr/internal/json"
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

// UnmarshalJSON implements custom unmarshaling for Section to handle dynamic data field types.
func (s *Section) UnmarshalJSON(data []byte) error {
	// Use a temporary struct with raw JSON for the data field
	var raw struct {
		Title           string          `json:"title"`
		Type            string          `json:"type,omitempty"`
		Description     string          `json:"description,omitempty"`
		ActionableTitle *ActionButton   `json:"actionableTitle,omitempty"`
		Button          *ActionButton   `json:"button,omitempty"`
		Data            json.RawMessage `json:"data,omitempty"`
		Action          *SectionAction  `json:"action,omitempty"`
		Steps           []StepItem      `json:"steps,omitempty"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal Section: %w", err)
	}

	s.Title = raw.Title
	s.Type = raw.Type
	s.Description = raw.Description
	s.ActionableTitle = raw.ActionableTitle
	s.Button = raw.Button
	s.Action = raw.Action
	s.Steps = raw.Steps

	// Handle the data field - can be array or object (skip objects)
	if len(raw.Data) > 0 {
		// Check if it's an array by looking at the first non-whitespace character
		trimmed := bytes.TrimSpace(raw.Data)
		if len(trimmed) > 0 && trimmed[0] == '[' {
			// It's an array, unmarshal normally
			var dataItems []SectionDataItem
			if err := json.Unmarshal(raw.Data, &dataItems); err != nil {
				return fmt.Errorf("failed to unmarshal Section.data: %w", err)
			}
			s.Data = dataItems
		} else if trimmed[0] == '{' {
			// It's an object, parse as HeaderData
			var headerData HeaderData
			if err := json.Unmarshal(raw.Data, &headerData); err != nil {
				return fmt.Errorf("failed to unmarshal Section.data as HeaderData: %w", err)
			}
			s.DataHeader = &headerData
		} else if len(trimmed) > 0 {
			return fmt.Errorf("unhandled Section.data type: %s", string(raw.Data))
		}
	}

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

// UnmarshalJSON implements custom unmarshaling with strict validation for SectionDataItem.
// It handles the case where detail can be either an object or a string.
func (sdi *SectionDataItem) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID          string          `json:"id,omitempty"`
		Title       string          `json:"title"`
		Style       string          `json:"style,omitempty"`
		Detail      json.RawMessage `json:"detail"`
		Action      *DetailAction   `json:"action,omitempty"`
		PostboxType string          `json:"postboxType,omitempty"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal SectionDataItem: %w", err)
	}

	sdi.ID = raw.ID
	sdi.Title = raw.Title
	sdi.Style = raw.Style
	sdi.Action = raw.Action

	// Handle the detail field - can be object or string
	if len(raw.Detail) > 0 {
		trimmed := bytes.TrimSpace(raw.Detail)
		if len(trimmed) > 0 && trimmed[0] == '{' {
			// It's an object, unmarshal normally
			var detail ItemDetail
			if err := json.Unmarshal(raw.Detail, &detail); err != nil {
				return fmt.Errorf("failed to unmarshal SectionDataItem.detail: %w", err)
			}
			sdi.Detail = detail
		} else if len(trimmed) > 0 && trimmed[0] == '"' {
			// It's a string, just store it in Detail.Text
			var textValue string
			if err := json.Unmarshal(raw.Detail, &textValue); err != nil {
				return fmt.Errorf("failed to unmarshal SectionDataItem.detail string: %w", err)
			}
			sdi.Detail = ItemDetail{Text: textValue}
		} else if len(trimmed) > 0 {
			return fmt.Errorf("unhandled SectionDataItem.detail type: %s", string(raw.Detail))
		}
	}

	return nil
}

// ItemDetail represents the detail information for a section data item.
type ItemDetail struct {
	Text            string           `json:"text"`
	Type            string           `json:"type,omitempty"`
	Icon            *IconValue       `json:"icon,omitempty"`
	DisplayValue    *DisplayValue    `json:"displayValue,omitempty"`
	FunctionalStyle string           `json:"functionalStyle,omitempty"`
	Action          *DetailAction    `json:"action,omitempty"`
	Trend           *string          `json:"trend,omitempty"`
	Style           string           `json:"style,omitempty"`
	Amount          string           `json:"amount,omitempty"`
	Status          string           `json:"status,omitempty"`
	Subtitle        string           `json:"subtitle,omitempty"`
	Title           string           `json:"title,omitempty"`
	Timestamp       string           `json:"timestamp,omitempty"`
	Content         *PayloadContent  `json:"content,omitempty"`
	Leading         *ItemLeading     `json:"leading,omitempty"`
	Trailing        *PayloadTrailing `json:"trailing,omitempty"`
}

// UnmarshalJSON implements custom unmarshaling with strict validation for ItemDetail.
// It handles the case where displayValue and icon can be either an object or a string.
func (id *ItemDetail) UnmarshalJSON(data []byte) error {
	// Use a temporary struct with raw JSON for polymorphic fields
	var raw struct {
		Text            string          `json:"text"`
		Type            string          `json:"type,omitempty"`
		Icon            json.RawMessage `json:"icon,omitempty"`
		DisplayValue    json.RawMessage `json:"displayValue,omitempty"`
		FunctionalStyle string          `json:"functionalStyle,omitempty"`
		Action          *DetailAction   `json:"action,omitempty"`
		Trend           *string         `json:"trend,omitempty"`
		Style           string          `json:"style,omitempty"`
		Amount          string          `json:"amount,omitempty"`
		Status          string          `json:"status,omitempty"`
		Subtitle        string          `json:"subtitle,omitempty"`
		Title           string          `json:"title,omitempty"`
		Timestamp       string          `json:"timestamp,omitempty"`
		Content         json.RawMessage `json:"content,omitempty"`
		Leading         json.RawMessage `json:"leading,omitempty"`
		Trailing        json.RawMessage `json:"trailing,omitempty"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal ItemDetail: %w", err)
	}

	id.Text = raw.Text
	id.Type = raw.Type
	id.FunctionalStyle = raw.FunctionalStyle
	id.Action = raw.Action
	id.Trend = raw.Trend
	id.Style = raw.Style
	id.Amount = raw.Amount
	id.Status = raw.Status
	id.Subtitle = raw.Subtitle
	id.Title = raw.Title
	id.Timestamp = raw.Timestamp

	// Handle the icon field - can be object or string
	if len(raw.Icon) > 0 && !bytes.Equal(bytes.TrimSpace(raw.Icon), []byte("null")) {
		var icon IconValue
		if err := json.Unmarshal(raw.Icon, &icon); err != nil {
			return fmt.Errorf("failed to unmarshal ItemDetail.icon: %w", err)
		}
		id.Icon = &icon
	}

	// Handle the displayValue field - can be object, string, or null
	if len(raw.DisplayValue) > 0 {
		trimmed := bytes.TrimSpace(raw.DisplayValue)
		if len(trimmed) > 0 && trimmed[0] == '{' {
			// It's an object, unmarshal normally
			var dv DisplayValue
			if err := json.Unmarshal(raw.DisplayValue, &dv); err != nil {
				return fmt.Errorf("failed to unmarshal ItemDetail.displayValue: %w", err)
			}
			id.DisplayValue = &dv
		} else if bytes.Equal(trimmed, []byte("null")) {
			// It's null, leave DisplayValue as nil
			id.DisplayValue = nil
		} else if len(trimmed) > 0 {
			return fmt.Errorf("unhandled ItemDetail.displayValue type: %s", string(raw.DisplayValue))
		}
	}

	// Handle the content field
	if len(raw.Content) > 0 && !bytes.Equal(bytes.TrimSpace(raw.Content), []byte("null")) {
		var content PayloadContent
		if err := json.Unmarshal(raw.Content, &content); err != nil {
			return fmt.Errorf("failed to unmarshal ItemDetail.content: %w", err)
		}
		id.Content = &content
	}

	// Handle the leading field
	if len(raw.Leading) > 0 && !bytes.Equal(bytes.TrimSpace(raw.Leading), []byte("null")) {
		var leading ItemLeading
		if err := json.Unmarshal(raw.Leading, &leading); err != nil {
			return fmt.Errorf("failed to unmarshal ItemDetail.leading: %w", err)
		}
		id.Leading = &leading
	}

	// Handle the trailing field
	if len(raw.Trailing) > 0 && !bytes.Equal(bytes.TrimSpace(raw.Trailing), []byte("null")) {
		var trailing PayloadTrailing
		if err := json.Unmarshal(raw.Trailing, &trailing); err != nil {
			return fmt.Errorf("failed to unmarshal ItemDetail.trailing: %w", err)
		}
		id.Trailing = &trailing
	}

	return nil
}

// DisplayValue represents additional display formatting information for an item.
type DisplayValue struct {
	Prefix string `json:"prefix,omitempty"`
	Text   string `json:"text,omitempty"`
}

// ActionButton represents an actionable button or title with an action and title.
type ActionButton struct {
	Action *DetailAction `json:"action,omitempty"`
	Title  string        `json:"title,omitempty"`
}

// StepItem represents a step in a timeline process.
type StepItem struct {
	Content StepContent `json:"content"`
	Leading StepLeading `json:"leading"`
}

// StepContent represents the content of a step.
type StepContent struct {
	CTA       *string `json:"cta,omitempty"`
	Subtitle  *string `json:"subtitle,omitempty"`
	Timestamp string  `json:"timestamp,omitempty"`
	Title     string  `json:"title,omitempty"`
}

// StepLeading represents the leading visual indicators of a step.
type StepLeading struct {
	Avatar     StepAvatar     `json:"avatar"`
	Connection StepConnection `json:"connection"`
}

// StepAvatar represents the visual avatar for a step.
type StepAvatar struct {
	Status string `json:"status"`
	Type   string `json:"type"`
}

// StepConnection represents the connection line between steps.
type StepConnection struct {
	Order string `json:"order"`
}

// DetailActionPayload represents the payload of a detail action.
// It can hold either a string value (for browserModal) or structured data.
type DetailActionPayload struct {
	// Common fields
	Value     string `json:"value,omitempty"`
	ID        string `json:"id,omitempty"`
	Type      string `json:"type,omitempty"`
	Text      string `json:"text,omitempty"`
	Icon      string `json:"icon,omitempty"`
	Title     string `json:"title,omitempty"`
	Path      string `json:"path,omitempty"`
	Shareable *bool  `json:"shareable,omitempty"`

	// Nested structures
	Sections     []NestedSection  `json:"sections,omitempty"`
	DisplayValue *DisplayValue    `json:"displayValue,omitempty"`
	Content      *PayloadContent  `json:"content,omitempty"`
	Trailing     *PayloadTrailing `json:"trailing,omitempty"`
	ChatAction   *ChatAction      `json:"chatAction,omitempty"`

	// Context parameters
	ContextCategory string        `json:"contextCategory,omitempty"`
	ContextParams   ContextParams `json:"contextParams,omitempty"`

	// Specific action fields
	SavingsPlanID           string   `json:"savingsPlanId,omitempty"`
	IBAN                    string   `json:"iban,omitempty"`
	InstrumentID            string   `json:"instrumentId,omitempty"`
	InterestPayoutID        string   `json:"interestPayoutId,omitempty"`
	TimelineEventID         string   `json:"timelineEventId,omitempty"`
	TransactionID           string   `json:"transactionId,omitempty"`
	TimelineEventTypes      []string `json:"timelineEventTypes,omitempty"`
	CashAnalyticsCategories []string `json:"cashAnalyticsCategories,omitempty"`
	ContactDetailsQuery     string   `json:"contactDetailsQuery,omitempty"`
	Link                    string   `json:"link,omitempty"`
}

// ContextParams represents context parameters for actions.
type ContextParams struct {
	Amount                     string `json:"amount,omitempty"`
	ChatFlowKey                string `json:"chat_flow_key,omitempty"`
	CreatedAt                  string `json:"createdAt,omitempty"`
	Currency                   string `json:"currency,omitempty"`
	CustomerSupportChatVisible string `json:"customerSupportChatVisible,omitempty"`
	TimelineEventID            string `json:"timelineEventId,omitempty"`
	CardDisputeTxID            string `json:"card-dispute-txId,omitempty"`
	TransferID                 string `json:"transferId,omitempty"`
	GroupID                    string `json:"groupId,omitempty"`
	PrimID                     string `json:"primId,omitempty"`
	InstrumentID               string `json:"instrumentId,omitempty"`
	InstrumentType             string `json:"instrumentType,omitempty"`
	IBAN                       string `json:"iban,omitempty"`
	SavingsPlanID              string `json:"savingsPlanId,omitempty"`
	InterestPayoutID           string `json:"interestPayoutId,omitempty"`
}

// PayloadContent represents the content structure in payloads.
type PayloadContent struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Truncate bool   `json:"truncate,omitempty"`
	Type     string `json:"type"`
}

// PayloadTrailing represents the trailing structure in payloads.
type PayloadTrailing struct {
	Type       string `json:"type"`
	Title      string `json:"title,omitempty"`
	TitleColor string `json:"titleColor,omitempty"`
}

// ItemLeading represents the leading visual indicators of an item.
type ItemLeading struct {
	Type  string     `json:"type"`
	Value string     `json:"value,omitempty"`
	Icon  *IconValue `json:"icon,omitempty"`
}

// ChatAction represents a chat action within a payload.
type ChatAction struct {
	Type    string              `json:"type"`
	Payload DetailActionPayload `json:"payload"`
}

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

// UnmarshalJSON implements custom unmarshaling for NestedSection to handle dynamic data field types.
func (ns *NestedSection) UnmarshalJSON(data []byte) error {
	// Use a temporary struct with raw JSON for the data field
	var raw struct {
		Title  string          `json:"title,omitempty"`
		Type   string          `json:"type"`
		Text   string          `json:"text,omitempty"`
		Style  string          `json:"style,omitempty"`
		Data   json.RawMessage `json:"data,omitempty"`
		Action *NestedAction   `json:"action,omitempty"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal NestedSection: %w", err)
	}

	ns.Title = raw.Title
	ns.Type = raw.Type
	ns.Text = raw.Text
	ns.Style = raw.Style
	ns.Action = raw.Action

	// Handle the data field - can be array or object
	if len(raw.Data) > 0 {
		// Check if it's an array by looking at the first non-whitespace character
		trimmed := bytes.TrimSpace(raw.Data)
		if len(trimmed) > 0 && trimmed[0] == '[' {
			// It's an array, unmarshal normally
			var dataItems []NestedSectionDataItem
			if err := json.Unmarshal(raw.Data, &dataItems); err != nil {
				return fmt.Errorf("failed to unmarshal NestedSection.data: %w", err)
			}
			ns.Data = dataItems
		} else if trimmed[0] == '{' {
			// It's an object, parse as NestedSectionDataObject
			var dataObject NestedSectionDataObject
			if err := json.Unmarshal(raw.Data, &dataObject); err != nil {
				return fmt.Errorf("failed to unmarshal NestedSection.data as NestedSectionDataObject: %w", err)
			}
			ns.DataObject = &dataObject
		} else if len(trimmed) > 0 {
			return fmt.Errorf("unhandled NestedSection.data type: %s", string(raw.Data))
		}
	}

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

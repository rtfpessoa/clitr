package types

import (
	"bytes"
	"encoding/json"
	"fmt"
)

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

// UnmarshalJSON handles the fields whose API values have more than one shape.
func (id *ItemDetail) UnmarshalJSON(data []byte) error {
	type alias ItemDetail
	*id = ItemDetail{}
	raw := struct {
		*alias
		Icon         json.RawMessage `json:"icon,omitempty"`
		DisplayValue json.RawMessage `json:"displayValue,omitempty"`
		Content      json.RawMessage `json:"content,omitempty"`
		Leading      json.RawMessage `json:"leading,omitempty"`
		Trailing     json.RawMessage `json:"trailing,omitempty"`
	}{alias: (*alias)(id)}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to unmarshal ItemDetail: %w", err)
	}
	if err := decodeOptionalValue(raw.Icon, &id.Icon); err != nil {
		return fmt.Errorf("failed to unmarshal ItemDetail.icon: %w", err)
	}
	if err := decodeItemDisplayValue(raw.DisplayValue, &id.DisplayValue); err != nil {
		return fmt.Errorf("failed to unmarshal ItemDetail.displayValue: %w", err)
	}
	if err := decodeOptionalValue(raw.Content, &id.Content); err != nil {
		return fmt.Errorf("failed to unmarshal ItemDetail.content: %w", err)
	}
	if err := decodeOptionalValue(raw.Leading, &id.Leading); err != nil {
		return fmt.Errorf("failed to unmarshal ItemDetail.leading: %w", err)
	}
	if err := decodeOptionalValue(raw.Trailing, &id.Trailing); err != nil {
		return fmt.Errorf("failed to unmarshal ItemDetail.trailing: %w", err)
	}
	return nil
}

func decodeOptionalValue[T any](raw json.RawMessage, target **T) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	*target = &value
	return nil
}

func decodeItemDisplayValue(raw json.RawMessage, target **DisplayValue) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if trimmed[0] != '{' {
		return fmt.Errorf("unhandled ItemDetail.displayValue type: %s", string(raw))
	}
	return decodeOptionalValue(raw, target)
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
	NHCTimelineEventID         string `json:"NHC_timelineEventId,omitempty"`

	// PayloadContent represents the content structure in payloads.
	TradeID string `json:"tradeId,omitempty"`
}

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

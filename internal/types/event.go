// Package types defines types and parsing logic for Trade Republic timeline events.
// It handles conversion of raw API events into structured Event objects with proper
// type inference, field parsing, and normalization.
package types

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rtfpessoa/clitr/internal/log"
	"go.uber.org/zap"
)

const (
	DefaultLanguage = "en"

	// Section Title-based prefix event types
	YouReceived = "you received"
	YouSent     = "you sent"
	YouSpent    = "you spent"
	From        = "from"
	Sender      = "sender"
	To          = "to"

	// Title-based event types
	Interest         = "interest"
	SavingsPlan      = "savings plan"
	CardRefund       = "card refund"
	CardPayment      = "card payment"
	CardVerification = "card verification"
	CashIn           = "cash in"

	// Subtitle-based event types
	Dividend       = "dividend"
	CashDividend   = "cash dividend"
	Saveback       = "saveback"
	BuyOrder       = "buy order"
	LimitBuyOrder  = "limit buy order"
	SellOrder      = "sell order"
	LimitSellOrder = "limit sell order"
	StopSellOrder  = "stop sell order"
	SavingExecuted = "saving executed"
	Repayment      = "repayment"
	Sent           = "sent"

	// Value parsing
	Fee    = "fee"
	Tax    = "tax"
	Taxes  = "taxes"
	Free   = "free"
	Shares = "shares"

	Overview    = "overview"
	Trade       = "trade"
	Transaction = "transaction"
	IBAN        = "iban"
	Merchant    = "merchant"
)

// Package-level compiled regexes for performance (used in hot paths)
var (
	isinRE       = regexp.MustCompile(`^[A-Z]{2}[A-Z0-9]{10}$`)
	floatCleanRE = regexp.MustCompile(`[^,.\d-]`)
)

// EventType represents the type of transaction event
type EventType string

const (
	// Primary event types
	EventTypeBuy       EventType = "BUY"
	EventTypeDeposit   EventType = "DEPOSIT"
	EventTypeDividend  EventType = "DIVIDEND"
	EventTypeInterest  EventType = "INTEREST"
	EventTypeRemoval   EventType = "REMOVAL"
	EventTypeSell      EventType = "SELL"
	EventTypeTaxRefund EventType = "TAX_REFUND"

	// Conditional event types
	EventTypeSaveback     EventType = "SAVEBACK"
	EventTypeTradeInvoice EventType = "TRADE_INVOICE"

	EventTypeCardVerification EventType = "CARD_VERIFICATION"

	TimestampLayout = "2006-01-02T15:04:05.000-0700"
)

// Event represents a parsed transaction event
type Event struct {
	ID        string // Event ID from API
	Date      time.Time
	Title     string
	EventType EventType
	ISIN      string
	Shares    *float64
	Value     *float64
	Fees      *float64
	Taxes     *float64
	Note      string
	Subtitle  string
	Status    string // EXECUTED, CANCELED, or other status from API
	// Additional context for note generation
	Sender    string
	Recipient string
	IBAN      string
}

// TimelineEvent represents a raw timeline event from the API
type TimelineEvent struct {
	ID                string     `json:"id"`
	Timestamp         string     `json:"timestamp"` // ISO 8601 format string
	Icon              string     `json:"icon"`
	Title             string     `json:"title"`
	Body              string     `json:"body"`
	Status            string     `json:"status"`
	EventType         string     `json:"eventType"`
	Subtitle          string     `json:"subtitle"`
	Amount            *Amount    `json:"amount"`
	Action            *Action    `json:"action"`
	Avatar            *Avatar    `json:"avatar"`
	Badge             *string    `json:"badge"`
	CashAccountNumber *string    `json:"cashAccountNumber"`
	Deleted           bool       `json:"deleted"`
	Hidden            bool       `json:"hidden"`
	SubAmount         *SubAmount `json:"subAmount"`
}

// SubAmount represents the amount in the original currency (ie: USD while amount is always in EUR)
type SubAmount struct {
	Currency       string  `json:"currency"`
	FractionDigits int     `json:"fractionDigits"`
	Value          float64 `json:"value"`
}

// Avatar represents the image of the transaction
type Avatar struct {
	Asset string  `json:"asset"`
	Badge *string `json:"badge"`
}

// Amount represents a monetary amount
type Amount struct {
	Value          float64 `json:"value"`
	Currency       string  `json:"currency"`
	FractionDigits int     `json:"fractionDigits"`
}

// Action represents an action associated with an event
type Action struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

// Section title-based prefix event type mappings
var sectionTitlePrefixesEventTypeMap = map[string]EventType{
	YouReceived: EventTypeDeposit,
	//YouSent:     EventTypeRemoval,
	YouSpent: EventTypeDeposit,
}

// Section title-based event type mappings
var sectionTitleEventTypeMap = map[string]EventType{
	CardRefund:       EventTypeDeposit,
	CardPayment:      EventTypeRemoval,
	CardVerification: EventTypeCardVerification,
	From:             EventTypeDeposit,
	Sender:           EventTypeDeposit,
	To:               EventTypeRemoval,
}

// Title-based event type mappings
var titleEventTypeMap = map[string]EventType{
	Interest:         EventTypeInterest,
	SavingsPlan:      EventTypeTradeInvoice,
	CardRefund:       EventTypeDeposit,
	CardPayment:      EventTypeRemoval,
	CardVerification: EventTypeCardVerification,
	CashIn:           EventTypeDeposit,
}

// Subtitle-based prefixes event type mappings
var subtitlePrefixesEventTypeMap = map[string]EventType{
	// Saveback
	Saveback: EventTypeSaveback,
}

// Subtitle-based event type mappings
var subtitleEventTypeMap = map[string]EventType{
	// Dividends
	Dividend:     EventTypeDividend,
	CashDividend: EventTypeDividend,

	// Removal
	Sent: EventTypeRemoval,

	// Saveback
	Saveback: EventTypeSaveback,

	// Trade invoices
	BuyOrder:       EventTypeTradeInvoice,
	LimitBuyOrder:  EventTypeTradeInvoice,
	SellOrder:      EventTypeTradeInvoice,
	LimitSellOrder: EventTypeTradeInvoice,
	StopSellOrder:  EventTypeTradeInvoice,
	SavingExecuted: EventTypeTradeInvoice,

	// Maturity/Redemption (bonds, options)
	Repayment: EventTypeSell,

	// Card verification
	CardVerification: EventTypeCardVerification,
}

// Event type mappings from Trade Republic to our types
var trEventTypeMap = map[string]EventType{
	// Deposits
	"account_transfer_incoming":               EventTypeDeposit,
	"incoming_transfer":                       EventTypeDeposit,
	"incoming_transfer_delegation":            EventTypeDeposit,
	"payment_inbound":                         EventTypeDeposit,
	"payment_inbound_apple_pay":               EventTypeDeposit,
	"payment_inbound_google_pay":              EventTypeDeposit,
	"payment_inbound_sepa_direct_debit":       EventTypeDeposit,
	"payment_inbound_credit_card":             EventTypeDeposit,
	"payment-service-in-payment-direct-debit": EventTypeDeposit,
	"card_refund":                             EventTypeDeposit,
	"card_successful_oct":                     EventTypeDeposit,
	"card_tr_refund":                          EventTypeDeposit,

	// Dividends
	"credit":                            EventTypeDividend,
	"ssp_corporate_action_invoice_cash": EventTypeDividend,

	// Interest
	"interest_payout":         EventTypeInterest,
	"interest_payout_created": EventTypeInterest,

	// Removals
	"outgoing_transfer":              EventTypeRemoval,
	"outgoing_transfer_delegation":   EventTypeRemoval,
	"payment_outbound":               EventTypeRemoval,
	"card_failed_transaction":        EventTypeRemoval,
	"card_order_billed":              EventTypeRemoval,
	"card_successful_atm_withdrawal": EventTypeRemoval,
	"card_successful_transaction":    EventTypeRemoval,
	"junior_p2p_transfer":            EventTypeRemoval,

	// Saveback
	"acquisition_trade_perk":      EventTypeSaveback,
	"benefits_saveback_execution": EventTypeSaveback,

	// Tax refunds
	"tax_correction":             EventTypeTaxRefund,
	"tax_refund":                 EventTypeTaxRefund,
	"ssp_tax_correction_invoice": EventTypeTaxRefund,

	// Trade invoices
	"order_executed":                  EventTypeTradeInvoice,
	"savings_plan_executed":           EventTypeTradeInvoice,
	"savings_plan_invoice_created":    EventTypeTradeInvoice,
	"trade_corrected":                 EventTypeTradeInvoice,
	"trade_invoice":                   EventTypeTradeInvoice,
	"benefits_spare_change_execution": EventTypeTradeInvoice,
	"trading_savingsplan_executed":    EventTypeTradeInvoice,
	"trading_trade_executed":          EventTypeTradeInvoice,
}

// isCardVerification checks if an event is a card verification by looking for the banner
func isCardVerification(event TimelineEvent, details TimelineDetails) bool {
	if event.Title == CardVerification || event.Subtitle == CardVerification {
		return true
	}

	for _, section := range details.Sections {
		// Card verification banner
		if strings.ToLower(section.Title) == CardVerification {
			return true
		}
	}

	return false
}

// inferEventType determines the event type from timeline event and details
func inferEventType(te TimelineEvent, details TimelineDetails) (EventType, error) {
	// Try explicit eventType field first
	if te.EventType != "" && te.EventType != "timeline_legacy_migrated_events" {
		if eventType, ok := trEventTypeMap[strings.ToLower(te.EventType)]; ok {
			return eventType, nil
		}
	}

	// Try title-based matching
	if eventType, ok := titleEventTypeMap[strings.ToLower(te.Title)]; ok {
		return eventType, nil
	}

	// Try subtitle-based matching
	if eventType, ok := subtitleEventTypeMap[strings.ToLower(te.Subtitle)]; ok {
		return eventType, nil
	}

	// Try subtitle-based prefix matching
	for prefix, eventType := range subtitlePrefixesEventTypeMap {
		if strings.HasPrefix(strings.ToLower(te.Subtitle), prefix) {
			return eventType, nil
		}
	}

	// Try inferring from detail sections
	eventType, err := inferFromDetailSections(details)
	if err != nil {
		return "", fmt.Errorf("skipped: unknown event type (id='%s', title='%s', subtitle='%s')", te.ID, te.Title, te.Subtitle)
	}

	return eventType, nil
}

// inferFromDetailSections tries to infer event type from detail section patterns
func inferFromDetailSections(details TimelineDetails) (EventType, error) {
	if len(details.Sections) == 0 {
		return "", errors.New("could not find details sections to infer event type")
	}

	// Look for "Overview" section
	for _, section := range details.Sections {
		// Try section title-based prefix matching
		for prefix, eventType := range sectionTitlePrefixesEventTypeMap {
			if strings.HasPrefix(strings.ToLower(section.Title), prefix) {
				return eventType, nil
			}
		}

		if strings.ToLower(section.Title) == Overview {
			for _, item := range section.Data {
				// Try section title-based matching
				if eventType, ok := sectionTitleEventTypeMap[strings.ToLower(item.Title)]; ok {
					return eventType, nil
				}
			}
		}
	}

	return "", errors.New("could not infer event type from details")
}

// ParseEvent converts a TimelineEvent into an Event
func ParseEvent(te TimelineEvent, details TimelineDetails) (*Event, error) {
	// Skip canceled events first
	if strings.ToLower(te.Status) == "canceled" {
		log.Debug("skipped: canceled event", zap.Any("event", te))
		return nil, nil
	}

	// Check for card verification banner in details
	if isCardVerification(te, details) {
		log.Debug("skipped: card verification event", zap.Any("event", te))
		return nil, nil
	}

	timestamp, err := time.Parse(TimestampLayout, te.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp %s: %w", te.Timestamp, err)
	}

	// Determine event type - try multiple sources
	eventType, err := inferEventType(te, details)
	if err != nil {
		return nil, err
	}

	event := &Event{
		ID:        te.ID,
		Date:      timestamp,
		Title:     te.Title,
		Subtitle:  te.Subtitle,
		EventType: eventType,
		Status:    te.Status,
	}

	// Parse value
	if te.Amount != nil && te.Amount.Value != 0 {
		event.Value = &te.Amount.Value
	}

	// Parse type-dependent fields
	switch eventType {
	case EventTypeTradeInvoice, EventTypeDividend, EventTypeSaveback:
		event.ISIN = parseISIN(te, details)
		parseSharesFeesTaxes(event, details)
	case EventTypeInterest:
		parseTaxes(event, details)
	case EventTypeDeposit, EventTypeRemoval:
		if strings.HasPrefix(te.EventType, "card_") {
			event.Note = te.EventType
		}
		// Parse sender/recipient information
		parseSenderRecipient(event, details)
	}

	return event, nil
}

// parseISIN extracts the ISIN from an event
func parseISIN(te TimelineEvent, details TimelineDetails) string {
	// Try to extract from icon URL
	icon := te.Icon
	if strings.Contains(icon, "/") {
		parts := strings.Split(icon, "/")
		for i, part := range parts {
			if i > 0 && i < len(parts)-1 {
				// ISIN is typically between slashes in the icon URL
				if len(part) >= 12 && isISIN(part) {
					return part
				}
			}
		}
	}

	// Try from details sections
	for _, section := range details.Sections {
		if section.Action != nil && section.Action.Type == "instrumentDetail" {
			return section.Action.Payload.Value
		}
	}

	return ""
}

// isISIN checks if a string looks like an ISIN
func isISIN(s string) bool {
	if len(s) != 12 {
		return false
	}
	// ISIN starts with 2 letters followed by 10 alphanumeric characters
	return isinRE.MatchString(s)
}

// parseSharesFeesTaxes extracts shares, fees, and taxes from details
func parseSharesFeesTaxes(event *Event, details TimelineDetails) {
	for _, section := range details.Sections {
		title := strings.ToLower(section.Title)

		// Look for transaction or overview sections
		if title == Transaction || title == Trade || title == Overview {
			for _, item := range section.Data {
				itemTitle := strings.ToLower(item.Title)

				switch itemTitle {
				case Shares:
					if val := parseFloatFromText(item.Detail.Text); val != nil {
						event.Shares = val
					}
				case Transaction:
					// For Saveback, shares are in the prefix field (e.g., "0.006539 x ")
					if event.Shares == nil && item.Detail.DisplayValue != nil {
						if item.Detail.DisplayValue.Prefix != "" {
							if val := parseFloatFromText(item.Detail.DisplayValue.Prefix); val != nil {
								event.Shares = val
							}
						}
					}
				case Fee:
					if item.Detail.Text == Free {
						zero := 0.0
						event.Fees = &zero
					} else if val := parseFloatFromText(item.Detail.Text); val != nil {
						event.Fees = val
					}
				case Taxes, Tax:
					if val := parseFloatFromText(item.Detail.Text); val != nil {
						event.Taxes = val
					}
				}
			}
		}
	}
}

// parseSenderRecipient extracts sender/recipient information for deposits/withdrawals
func parseSenderRecipient(event *Event, details TimelineDetails) {
	for _, section := range details.Sections {
		if strings.ToLower(section.Title) == Overview {
			for _, item := range section.Data {
				itemTitle := strings.ToLower(item.Title)

				switch itemTitle {
				case From, Sender:
					event.Sender = item.Detail.Text
				case To:
					event.Recipient = item.Detail.Text
				case IBAN:
					event.IBAN = item.Detail.Text
				case Merchant:
					// For card transactions, capture merchant if different from title
					if item.Detail.Text != "" && item.Detail.Text != event.Title {
						event.Note = item.Detail.Text
					}
				}
			}
		}
	}
}

// parseTaxes extracts taxes from event details
func parseTaxes(event *Event, details TimelineDetails) {
	for _, section := range details.Sections {
		title := strings.ToLower(section.Title)

		if title == Transaction || title == Trade || title == Overview {
			for _, item := range section.Data {
				itemTitle := strings.ToLower(item.Title)

				if itemTitle == Tax || itemTitle == Taxes {
					if val := parseFloatFromText(item.Detail.Text); val != nil {
						event.Taxes = val
					}
				}
			}
		}
	}
}

// parseFloatFromText extracts a float value from a text string
func parseFloatFromText(text string) *float64 {
	// Remove all non-numeric characters except comma, dot, and minus
	cleaned := floatCleanRE.ReplaceAllString(text, "")

	if cleaned == "" {
		return nil
	}

	// Try parsing as German format first (1.234,56)
	if strings.Contains(cleaned, ",") {
		cleaned = strings.ReplaceAll(cleaned, ".", "")
		cleaned = strings.ReplaceAll(cleaned, ",", ".")
	}

	val, err := strconv.ParseFloat(cleaned, 64)
	if err != nil || val == 0 {
		return nil
	}

	return &val
}

// GetTransactionType returns the final transaction type for export
func (e *Event) GetTransactionType() EventType {
	switch e.EventType {
	case EventTypeTradeInvoice:
		// Buy if value is negative (money out), Sell if positive (money in)
		if e.Value != nil && *e.Value < 0 {
			return EventTypeBuy
		}
		return EventTypeSell
	case EventTypeSaveback:
		// Same logic as trade invoice
		if e.Value != nil && *e.Value < 0 {
			return EventTypeBuy
		}
		return EventTypeSell
	default:
		return e.EventType
	}
}

// GetNote returns the note field for export with comprehensive details
// Pass translator to translate German terms to target language
func (e *Event) GetNote() string {
	var parts []string

	// 1. Most important: Subtitle (transaction context) - translated
	if e.Subtitle != "" {
		parts = append(parts, e.Subtitle)
	}

	// 2. Title (security name or merchant) - translate if it's a German term, otherwise keep as-is
	translatedTitle := e.Title
	parts = append(parts, translatedTitle)

	// 3. Sender/Recipient information (for transfers)
	if e.Sender != "" {
		parts = append(parts, fmt.Sprintf("From: %s", e.Sender))
	}
	if e.Recipient != "" {
		parts = append(parts, fmt.Sprintf("To: %s", e.Recipient))
	}
	if e.IBAN != "" {
		parts = append(parts, fmt.Sprintf("IBAN: %s", e.IBAN))
	}

	// 4. Additional note (card event types, merchant info) - translated
	if e.Note != "" && e.Note != e.Title {
		parts = append(parts, e.Note)
	}

	return strings.Join(parts, " | ")
}

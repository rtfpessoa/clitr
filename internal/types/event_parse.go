package types

import (
	"fmt"
	"strings"
	"time"

	"github.com/rtfpessoa/clitr/internal/log"
	"go.uber.org/zap"
)

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

	parseEventDetails(event, te, details)
	return event, nil
}

func parseEventDetails(event *Event, te TimelineEvent, details TimelineDetails) {
	switch event.EventType {
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

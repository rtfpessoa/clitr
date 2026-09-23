package types

import (
	"strconv"
	"strings"
)

const isinLength = 12

// parseISIN extracts the ISIN from an event
func parseISIN(te TimelineEvent, details TimelineDetails) string {
	// Try to extract from icon URL
	parts := strings.Split(te.Icon, "/")
	for i := 1; i < len(parts)-1; i++ {
		if isISIN(parts[i]) {
			return parts[i]
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
	if len(s) != isinLength {
		return false
	}
	// ISIN starts with 2 letters followed by 10 alphanumeric characters
	return isinRE.MatchString(s)
}

// parseSharesFeesTaxes extracts shares, fees, and taxes from details
func parseSharesFeesTaxes(event *Event, details TimelineDetails) {
	for _, section := range details.Sections {
		if !isTradeSection(section.Title) {
			continue
		}
		for _, item := range section.Data {
			applyTradeDetail(event, item)
		}
	}
}

func isTradeSection(title string) bool {
	switch strings.ToLower(title) {
	case Transaction, Trade, Overview:
		return true
	default:
		return false
	}
}

func applyTradeDetail(event *Event, item SectionDataItem) {
	switch strings.ToLower(item.Title) {
	case Shares:
		if value := parseFloatFromText(item.Detail.Text); value != nil {
			event.Shares = value
		}
	case Transaction:
		// Saveback shares can appear in the display prefix.
		if event.Shares == nil && item.Detail.DisplayValue != nil {
			event.Shares = parseFloatFromText(item.Detail.DisplayValue.Prefix)
		}
	case Fee:
		if item.Detail.Text == Free {
			zero := 0.0
			event.Fees = &zero
		} else if value := parseFloatFromText(item.Detail.Text); value != nil {
			event.Fees = value
		}
	case Taxes, Tax:
		if value := parseFloatFromText(item.Detail.Text); value != nil {
			event.Taxes = value
		}
	}
}

// parseSenderRecipient extracts sender/recipient information for deposits/withdrawals
func parseSenderRecipient(event *Event, details TimelineDetails) {
	for _, section := range details.Sections {
		if strings.ToLower(section.Title) != Overview {
			continue
		}
		for _, item := range section.Data {
			applyTransferDetail(event, item)
		}
	}
}

func applyTransferDetail(event *Event, item SectionDataItem) {
	switch strings.ToLower(item.Title) {
	case From, Sender:
		event.Sender = item.Detail.Text
	case To:
		event.Recipient = item.Detail.Text
	case IBAN:
		event.IBAN = item.Detail.Text
	case Merchant:
		if item.Detail.Text != "" && item.Detail.Text != event.Title {
			event.Note = item.Detail.Text
		}
	}
}

// parseTaxes extracts taxes from event details
func parseTaxes(event *Event, details TimelineDetails) {
	for _, section := range details.Sections {
		if !isTradeSection(section.Title) {
			continue
		}
		for _, item := range section.Data {
			itemTitle := strings.ToLower(item.Title)
			if itemTitle == Tax || itemTitle == Taxes {
				if value := parseFloatFromText(item.Detail.Text); value != nil {
					event.Taxes = value
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
	if err != nil {
		return nil
	}

	return &val
}

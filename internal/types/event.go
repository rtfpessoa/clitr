// Package types defines Trade Republic timeline event types.
package types

import (
	"regexp"
	"time"
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

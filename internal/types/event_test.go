package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEvent_BuyOrder(t *testing.T) {
	te := TimelineEvent{
		ID:        "buy-001",
		Timestamp: "2024-01-15T10:30:00.000+0100",
		Icon:      "https://assets.traderepublic.com/img/logos/US0378331005/light.min.svg",
		Title:     "Apple Inc.",
		Status:    "EXECUTED",
		EventType: "order_executed",
		Subtitle:  "Buy Order",
		Amount:    &Amount{Value: -150.50, Currency: "EUR"},
	}

	details := TimelineDetails{
		ID: "buy-001",
		Sections: []Section{
			{
				Title: "Transaction",
				Type:  "table",
				Data: []SectionDataItem{
					{Title: "Shares", Detail: ItemDetail{Text: "1.5"}},
					{Title: "Fee", Detail: ItemDetail{Text: "0.50 EUR"}},
				},
			},
		},
	}

	event, err := ParseEvent(te, details)
	require.NoError(t, err)
	require.NotNil(t, event)

	assert.Equal(t, "buy-001", event.ID)
	assert.Equal(t, EventTypeTradeInvoice, event.EventType)
	assert.Equal(t, "Apple Inc.", event.Title)
	assert.Equal(t, "US0378331005", event.ISIN)
	require.NotNil(t, event.Shares)
	assert.Equal(t, 1.5, *event.Shares)
	require.NotNil(t, event.Fees)
	assert.Equal(t, 0.50, *event.Fees)
}

func TestParseEvent_SellOrder(t *testing.T) {
	te := TimelineEvent{
		ID:        "sell-001",
		Timestamp: "2024-02-20T14:45:00.000+0100",
		Icon:      "https://assets.traderepublic.com/img/logos/DE0007164600/light.min.svg",
		Title:     "SAP SE",
		Status:    "EXECUTED",
		EventType: "order_executed",
		Subtitle:  "Sell Order",
		Amount:    &Amount{Value: 250.75, Currency: "EUR"},
	}

	details := TimelineDetails{
		ID: "sell-001",
		Sections: []Section{
			{
				Title: "Transaction",
				Type:  "table",
				Data: []SectionDataItem{
					{Title: "Shares", Detail: ItemDetail{Text: "2"}},
					{Title: "Fee", Detail: ItemDetail{Text: "1.00 EUR"}},
					{Title: "Taxes", Detail: ItemDetail{Text: "0.25 EUR"}},
				},
			},
		},
	}

	event, err := ParseEvent(te, details)
	require.NoError(t, err)
	require.NotNil(t, event)

	assert.Equal(t, EventTypeTradeInvoice, event.EventType)
	require.NotNil(t, event.Taxes)
	assert.Equal(t, 0.25, *event.Taxes)
}

func TestParseEvent_Dividend(t *testing.T) {
	te := TimelineEvent{
		ID:        "div-001",
		Timestamp: "2024-03-15T08:00:00.000+0100",
		Icon:      "https://assets.traderepublic.com/img/logos/US5949181045/light.min.svg",
		Title:     "Microsoft Corp.",
		Status:    "EXECUTED",
		EventType: "ssp_corporate_action_invoice_cash",
		Subtitle:  "Dividend",
		Amount:    &Amount{Value: 12.50, Currency: "EUR"},
	}

	details := TimelineDetails{
		ID: "div-001",
		Sections: []Section{
			{
				Title: "Overview",
				Type:  "table",
				Data: []SectionDataItem{
					{Title: "Taxes", Detail: ItemDetail{Text: "-2.50 EUR"}},
				},
			},
		},
	}

	event, err := ParseEvent(te, details)
	require.NoError(t, err)
	require.NotNil(t, event)

	assert.Equal(t, EventTypeDividend, event.EventType)
	assert.Equal(t, "US5949181045", event.ISIN)
}

func TestParseEvent_CanceledReturnsNil(t *testing.T) {
	te := TimelineEvent{
		ID:        "cancel-001",
		Timestamp: "2024-01-20T16:30:00.000+0100",
		Title:     "Tesla Inc.",
		Status:    "CANCELED",
		EventType: "order_executed",
		Subtitle:  "Buy Order",
	}

	details := TimelineDetails{ID: "cancel-001"}

	event, err := ParseEvent(te, details)
	require.NoError(t, err)
	assert.Nil(t, event)
}

func TestInferEventType_FromEventType(t *testing.T) {
	te := TimelineEvent{
		EventType: "order_executed",
	}
	details := TimelineDetails{}

	eventType, err := inferEventType(te, details)
	require.NoError(t, err)
	assert.Equal(t, EventTypeTradeInvoice, eventType)
}

func TestInferEventType_FromTitle(t *testing.T) {
	te := TimelineEvent{
		Title: "Interest",
	}
	details := TimelineDetails{}

	eventType, err := inferEventType(te, details)
	require.NoError(t, err)
	assert.Equal(t, EventTypeInterest, eventType)
}

func TestInferEventType_FromSubtitle(t *testing.T) {
	te := TimelineEvent{
		Subtitle: "Dividend",
	}
	details := TimelineDetails{}

	eventType, err := inferEventType(te, details)
	require.NoError(t, err)
	assert.Equal(t, EventTypeDividend, eventType)
}

func TestParseISIN_FromIcon(t *testing.T) {
	te := TimelineEvent{
		Icon: "https://assets.traderepublic.com/img/logos/US0378331005/light.min.svg",
	}
	details := TimelineDetails{}

	isin := parseISIN(te, details)
	assert.Equal(t, "US0378331005", isin)
}

func TestParseISIN_FromSectionAction(t *testing.T) {
	te := TimelineEvent{
		Icon: "saveback",
	}
	details := TimelineDetails{
		Sections: []Section{
			{
				Action: &SectionAction{
					Type: "instrumentDetail",
					Payload: ActionPayload{
						Value: "IE00B4L5Y983",
					},
				},
			},
		},
	}

	isin := parseISIN(te, details)
	assert.Equal(t, "IE00B4L5Y983", isin)
}

func TestParseSharesFeesTaxes_AllFields(t *testing.T) {
	event := &Event{}
	details := TimelineDetails{
		Sections: []Section{
			{
				Title: "Transaction",
				Data: []SectionDataItem{
					{Title: "Shares", Detail: ItemDetail{Text: "10.5"}},
					{Title: "Fee", Detail: ItemDetail{Text: "1.00 EUR"}},
					{Title: "Taxes", Detail: ItemDetail{Text: "2.50 EUR"}},
				},
			},
		},
	}

	parseSharesFeesTaxes(event, details)

	require.NotNil(t, event.Shares)
	assert.Equal(t, 10.5, *event.Shares)
	require.NotNil(t, event.Fees)
	assert.Equal(t, 1.0, *event.Fees)
	require.NotNil(t, event.Taxes)
	assert.Equal(t, 2.5, *event.Taxes)
}

func TestParseSharesFeesTaxes_FreeFee(t *testing.T) {
	event := &Event{}
	details := TimelineDetails{
		Sections: []Section{
			{
				Title: "Transaction",
				Data: []SectionDataItem{
					{Title: "Fee", Detail: ItemDetail{Text: "free"}},
				},
			},
		},
	}

	parseSharesFeesTaxes(event, details)

	require.NotNil(t, event.Fees)
	assert.Equal(t, 0.0, *event.Fees)
}

func TestParseFloatFromText_GermanFormat(t *testing.T) {
	result := parseFloatFromText("1.234,56 EUR")
	require.NotNil(t, result)
	assert.Equal(t, 1234.56, *result)
}

func TestParseFloatFromText_USFormat(t *testing.T) {
	result := parseFloatFromText("1234.56 USD")
	require.NotNil(t, result)
	assert.Equal(t, 1234.56, *result)
}

func TestGetTransactionType_BuySell(t *testing.T) {
	negativeValue := -100.0
	positiveValue := 100.0

	buyEvent := Event{
		EventType: EventTypeTradeInvoice,
		Value:     &negativeValue,
	}
	assert.Equal(t, EventTypeBuy, buyEvent.GetTransactionType())

	sellEvent := Event{
		EventType: EventTypeTradeInvoice,
		Value:     &positiveValue,
	}
	assert.Equal(t, EventTypeSell, sellEvent.GetTransactionType())
}

package types

import (
	"testing"

	"github.com/rtfpessoa/clitr/internal/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIconValue_UnmarshalString(t *testing.T) {
	data := []byte(`"deposit"`)

	var iv IconValue
	err := json.Unmarshal(data, &iv)
	require.NoError(t, err)

	assert.Equal(t, "deposit", iv.Value)
	assert.Empty(t, iv.Type)
	assert.Empty(t, iv.URI)
}

func TestIconValue_UnmarshalObject(t *testing.T) {
	data := []byte(`{"type": "image", "uri": "https://example.com/icon.png", "badge": "new"}`)

	var iv IconValue
	err := json.Unmarshal(data, &iv)
	require.NoError(t, err)

	assert.Empty(t, iv.Value)
	assert.Equal(t, "image", iv.Type)
	assert.Equal(t, "https://example.com/icon.png", iv.URI)
	assert.Equal(t, "new", iv.Badge)
}

func TestSection_DataArray(t *testing.T) {
	data := []byte(`{
		"title": "Transaction",
		"type": "table",
		"data": [
			{"title": "Shares", "detail": {"text": "1.5", "type": "text"}},
			{"title": "Price", "detail": {"text": "100 EUR", "type": "text"}}
		]
	}`)

	var s Section
	err := json.Unmarshal(data, &s)
	require.NoError(t, err)

	assert.Equal(t, "Transaction", s.Title)
	assert.Equal(t, "table", s.Type)
	assert.Len(t, s.Data, 2)
	assert.Equal(t, "Shares", s.Data[0].Title)
	assert.Nil(t, s.DataHeader)
}

func TestSection_DataObject(t *testing.T) {
	data := []byte(`{
		"title": "Header",
		"type": "header",
		"data": {
			"icon": "deposit",
			"status": "completed",
			"text": "Transfer received"
		}
	}`)

	var s Section
	err := json.Unmarshal(data, &s)
	require.NoError(t, err)

	assert.Equal(t, "Header", s.Title)
	require.NotNil(t, s.DataHeader)
	assert.Equal(t, "completed", s.DataHeader.Status)
	assert.Equal(t, "Transfer received", s.DataHeader.Text)
	assert.Empty(t, s.Data)
}

func TestSectionDataItem_DetailObject(t *testing.T) {
	data := []byte(`{
		"title": "Amount",
		"detail": {"text": "150.00 EUR", "type": "text"}
	}`)

	var sdi SectionDataItem
	err := json.Unmarshal(data, &sdi)
	require.NoError(t, err)

	assert.Equal(t, "Amount", sdi.Title)
	assert.Equal(t, "150.00 EUR", sdi.Detail.Text)
	assert.Equal(t, "text", sdi.Detail.Type)
}

func TestSectionDataItem_DetailString(t *testing.T) {
	data := []byte(`{
		"title": "Status",
		"detail": "Completed"
	}`)

	var sdi SectionDataItem
	err := json.Unmarshal(data, &sdi)
	require.NoError(t, err)

	assert.Equal(t, "Status", sdi.Title)
	assert.Equal(t, "Completed", sdi.Detail.Text)
}

func TestItemDetail_WithDisplayValue(t *testing.T) {
	data := []byte(`{
		"text": "100.00 EUR",
		"type": "text",
		"displayValue": {"prefix": "+", "text": "100.00"}
	}`)

	var id ItemDetail
	err := json.Unmarshal(data, &id)
	require.NoError(t, err)

	assert.Equal(t, "100.00 EUR", id.Text)
	require.NotNil(t, id.DisplayValue)
	assert.Equal(t, "+", id.DisplayValue.Prefix)
	assert.Equal(t, "100.00", id.DisplayValue.Text)
}

func TestItemDetail_NullDisplayValue(t *testing.T) {
	data := []byte(`{
		"text": "Simple text",
		"displayValue": null
	}`)

	var id ItemDetail
	err := json.Unmarshal(data, &id)
	require.NoError(t, err)

	assert.Equal(t, "Simple text", id.Text)
	assert.Nil(t, id.DisplayValue)
}

func TestDetailActionPayload_String(t *testing.T) {
	data := []byte(`"https://example.com/document.pdf"`)

	var dap DetailActionPayload
	err := json.Unmarshal(data, &dap)
	require.NoError(t, err)

	assert.Equal(t, "https://example.com/document.pdf", dap.Value)
}

func TestDetailActionPayload_Object(t *testing.T) {
	data := []byte(`{
		"instrumentId": "US0378331005",
		"contextCategory": "trade"
	}`)

	var dap DetailActionPayload
	err := json.Unmarshal(data, &dap)
	require.NoError(t, err)

	assert.Empty(t, dap.Value)
	assert.Equal(t, "US0378331005", dap.InstrumentID)
	assert.Equal(t, "trade", dap.ContextCategory)
}

func TestDetailActionPayload_Path(t *testing.T) {
	data := []byte(`{
		"path": "api/v1/card/transactions/timelineDocuments/abc",
		"shareable": true
	}`)

	var dap DetailActionPayload
	err := json.Unmarshal(data, &dap)
	require.NoError(t, err)

	assert.Equal(t, "api/v1/card/transactions/timelineDocuments/abc", dap.Path)
	require.NotNil(t, dap.Shareable)
	assert.True(t, *dap.Shareable)
}

func TestDetailAction_DisplayMode(t *testing.T) {
	data := []byte(`{
		"type": "infoPage",
		"displayMode": "inline",
		"payload": {"value": "test"}
	}`)

	var da DetailAction
	err := json.Unmarshal(data, &da)
	require.NoError(t, err)

	assert.Equal(t, "infoPage", da.Type)
	assert.Equal(t, "inline", da.DisplayMode)
	assert.Equal(t, "test", da.Payload.Value)
}

func TestNestedSection_DataArray(t *testing.T) {
	data := []byte(`{
		"title": "Details",
		"type": "table",
		"data": [
			{"title": "Fee", "detail": {"text": "0.50 EUR"}}
		]
	}`)

	var ns NestedSection
	err := json.Unmarshal(data, &ns)
	require.NoError(t, err)

	assert.Equal(t, "Details", ns.Title)
	assert.Len(t, ns.Data, 1)
	assert.Nil(t, ns.DataObject)
}

func TestNestedSection_DataObject(t *testing.T) {
	data := []byte(`{
		"title": "Action",
		"type": "actionButtons",
		"data": {
			"title": "View details",
			"action": {
				"type": "openUrl",
				"payload": {"value": "https://example.com"}
			}
		}
	}`)

	var ns NestedSection
	err := json.Unmarshal(data, &ns)
	require.NoError(t, err)

	assert.Equal(t, "Action", ns.Title)
	require.NotNil(t, ns.DataObject)
	assert.Equal(t, "View details", ns.DataObject.Title)
	assert.Empty(t, ns.Data)
}

func TestNestedSection_DataObjectStatus(t *testing.T) {
	data := []byte(`{
		"title": "Header",
		"type": "header",
		"data": {
			"status": "executed",
			"subtitleText": "December 2025"
		}
	}`)

	var ns NestedSection
	err := json.Unmarshal(data, &ns)
	require.NoError(t, err)

	require.NotNil(t, ns.DataObject)
	assert.Equal(t, "executed", ns.DataObject.Status)
	assert.Equal(t, "December 2025", ns.DataObject.SubtitleText)
}

func TestPayloadContent_Subtitle(t *testing.T) {
	data := []byte(`{
		"title": "1 % Saveback",
		"subtitle": "No eligible spend",
		"type": "default"
	}`)

	var pc PayloadContent
	err := json.Unmarshal(data, &pc)
	require.NoError(t, err)

	assert.Equal(t, "1 % Saveback", pc.Title)
	assert.Equal(t, "No eligible spend", pc.Subtitle)
	assert.Equal(t, "default", pc.Type)
}

func TestItemDetail_WithContentLeadingTrailing(t *testing.T) {
	data := []byte(`{
		"content": {
			"title": "Transaction",
			"subtitle": "Payment received",
			"type": "default"
		},
		"leading": {
			"type": "date",
			"value": "2024-10-01"
		},
		"trailing": {
			"title": "+€3.66",
			"titleColor": "default",
			"type": "value"
		},
		"type": "listItem"
	}`)

	var id ItemDetail
	err := json.Unmarshal(data, &id)
	require.NoError(t, err)

	assert.Equal(t, "listItem", id.Type)

	require.NotNil(t, id.Content)
	assert.Equal(t, "Transaction", id.Content.Title)
	assert.Equal(t, "Payment received", id.Content.Subtitle)
	assert.Equal(t, "default", id.Content.Type)

	require.NotNil(t, id.Leading)
	assert.Equal(t, "date", id.Leading.Type)
	assert.Equal(t, "2024-10-01", id.Leading.Value)

	require.NotNil(t, id.Trailing)
	assert.Equal(t, "+€3.66", id.Trailing.Title)
	assert.Equal(t, "default", id.Trailing.TitleColor)
	assert.Equal(t, "value", id.Trailing.Type)
}

func TestItemDetail_WithLeading(t *testing.T) {
	data := []byte(`{
		"text": "Order details",
		"type": "text",
		"leading": {
			"type": "icon",
			"value": "shopping_cart"
		}
	}`)

	var id ItemDetail
	err := json.Unmarshal(data, &id)
	require.NoError(t, err)

	assert.Equal(t, "Order details", id.Text)
	assert.Equal(t, "text", id.Type)

	require.NotNil(t, id.Leading)
	assert.Equal(t, "icon", id.Leading.Type)
	assert.Equal(t, "shopping_cart", id.Leading.Value)

	assert.Nil(t, id.Content)
	assert.Nil(t, id.Trailing)
}

func TestPayloadTrailing_WithTitleColor(t *testing.T) {
	data := []byte(`{
		"type": "value",
		"title": "+€10.00",
		"titleColor": "green"
	}`)

	var pt PayloadTrailing
	err := json.Unmarshal(data, &pt)
	require.NoError(t, err)

	assert.Equal(t, "value", pt.Type)
	assert.Equal(t, "+€10.00", pt.Title)
	assert.Equal(t, "green", pt.TitleColor)
}

func TestNestedItemDetail_WithContentLeadingTrailing(t *testing.T) {
	data := []byte(`{
		"content": {
			"title": "€46,549.68 balance",
			"subtitle": "2.00% annual rate",
			"type": "default"
		},
		"leading": {
			"type": "date",
			"value": "2025-12-01"
		},
		"trailing": {
			"title": "+€2.55",
			"titleColor": "default",
			"type": "value"
		},
		"type": "listItem"
	}`)

	var nid NestedItemDetail
	err := json.Unmarshal(data, &nid)
	require.NoError(t, err)

	assert.Equal(t, "listItem", nid.Type)

	require.NotNil(t, nid.Content)
	assert.Equal(t, "€46,549.68 balance", nid.Content.Title)
	assert.Equal(t, "2.00% annual rate", nid.Content.Subtitle)
	assert.Equal(t, "default", nid.Content.Type)

	require.NotNil(t, nid.Leading)
	assert.Equal(t, "date", nid.Leading.Type)
	assert.Equal(t, "2025-12-01", nid.Leading.Value)

	require.NotNil(t, nid.Trailing)
	assert.Equal(t, "+€2.55", nid.Trailing.Title)
	assert.Equal(t, "default", nid.Trailing.TitleColor)
	assert.Equal(t, "value", nid.Trailing.Type)
}

func TestItemLeading_WithIcon(t *testing.T) {
	data := []byte(`{
		"type": "avatar",
		"icon": "account_balance"
	}`)

	var il ItemLeading
	err := json.Unmarshal(data, &il)
	require.NoError(t, err)

	assert.Equal(t, "avatar", il.Type)
	require.NotNil(t, il.Icon)
	assert.Equal(t, "account_balance", il.Icon.Value)
}

func TestItemLeading_WithIconObject(t *testing.T) {
	data := []byte(`{
		"type": "avatar",
		"icon": {
			"type": "image",
			"uri": "https://example.com/icon.png"
		}
	}`)

	var il ItemLeading
	err := json.Unmarshal(data, &il)
	require.NoError(t, err)

	assert.Equal(t, "avatar", il.Type)
	require.NotNil(t, il.Icon)
	assert.Equal(t, "image", il.Icon.Type)
	assert.Equal(t, "https://example.com/icon.png", il.Icon.URI)
}

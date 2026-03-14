package patch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/rtfpessoa/clitr/internal/types"
)

// structInfo holds metadata about a Go struct for unknown field detection.
type structInfo struct {
	name     string
	file     string
	jsonTags map[string]bool // known JSON tag names
}

// childMapping describes how a parent struct's JSON field maps to a child struct.
type childMapping struct {
	jsonKey      string
	childInfo    structInfo
	isArray      bool
	children     []childMapping // recursive children
	// altChildInfo is used for polymorphic fields (e.g., Section.data can be []SectionDataItem or HeaderData).
	// When isArray is true and the JSON value is an object, altChildInfo is used instead.
	altChildInfo *structInfo
	altChildren  []childMapping
}

// buildStructInfo creates a structInfo from a reflect.Type, extracting all json tags.
func buildStructInfo(name, file string, t reflect.Type) structInfo {
	tags := make(map[string]bool)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		jsonName := strings.Split(tag, ",")[0]
		if jsonName != "" {
			tags[jsonName] = true
		}
	}
	return structInfo{name: name, file: file, jsonTags: tags}
}

// typeTree defines the JSON-to-Go-struct hierarchy for RawEvent.
// This is built once and reused for all detections.
var typeTree = buildTypeTree()

func buildTypeTree() childMapping {
	rawEventInfo := buildStructInfo("RawEvent", "internal/types/raw.go", reflect.TypeOf(types.RawEvent{}))
	timelineEventInfo := buildStructInfo("TimelineEvent", "internal/types/event.go", reflect.TypeOf(types.TimelineEvent{}))
	timelineDetailsInfo := buildStructInfo("TimelineDetails", "internal/types/raw.go", reflect.TypeOf(types.TimelineDetails{}))
	sectionInfo := buildStructInfo("Section", "internal/types/raw.go", reflect.TypeOf(types.Section{}))
	headerDataInfo := buildStructInfo("HeaderData", "internal/types/raw.go", reflect.TypeOf(types.HeaderData{}))
	sectionDataItemInfo := buildStructInfo("SectionDataItem", "internal/types/raw.go", reflect.TypeOf(types.SectionDataItem{}))
	itemDetailInfo := buildStructInfo("ItemDetail", "internal/types/raw.go", reflect.TypeOf(types.ItemDetail{}))
	displayValueInfo := buildStructInfo("DisplayValue", "internal/types/raw.go", reflect.TypeOf(types.DisplayValue{}))
	actionButtonInfo := buildStructInfo("ActionButton", "internal/types/raw.go", reflect.TypeOf(types.ActionButton{}))
	stepItemInfo := buildStructInfo("StepItem", "internal/types/raw.go", reflect.TypeOf(types.StepItem{}))
	stepContentInfo := buildStructInfo("StepContent", "internal/types/raw.go", reflect.TypeOf(types.StepContent{}))
	stepLeadingInfo := buildStructInfo("StepLeading", "internal/types/raw.go", reflect.TypeOf(types.StepLeading{}))
	stepAvatarInfo := buildStructInfo("StepAvatar", "internal/types/raw.go", reflect.TypeOf(types.StepAvatar{}))
	stepConnectionInfo := buildStructInfo("StepConnection", "internal/types/raw.go", reflect.TypeOf(types.StepConnection{}))
	detailActionInfo := buildStructInfo("DetailAction", "internal/types/raw.go", reflect.TypeOf(types.DetailAction{}))
	detailActionPayloadInfo := buildStructInfo("DetailActionPayload", "internal/types/raw.go", reflect.TypeOf(types.DetailActionPayload{}))
	sectionActionInfo := buildStructInfo("SectionAction", "internal/types/raw.go", reflect.TypeOf(types.SectionAction{}))
	actionPayloadInfo := buildStructInfo("ActionPayload", "internal/types/raw.go", reflect.TypeOf(types.ActionPayload{}))
	contextParamsInfo := buildStructInfo("ContextParams", "internal/types/raw.go", reflect.TypeOf(types.ContextParams{}))
	payloadContentInfo := buildStructInfo("PayloadContent", "internal/types/raw.go", reflect.TypeOf(types.PayloadContent{}))
	payloadTrailingInfo := buildStructInfo("PayloadTrailing", "internal/types/raw.go", reflect.TypeOf(types.PayloadTrailing{}))
	itemLeadingInfo := buildStructInfo("ItemLeading", "internal/types/raw.go", reflect.TypeOf(types.ItemLeading{}))
	iconValueInfo := buildStructInfo("IconValue", "internal/types/raw.go", reflect.TypeOf(types.IconValue{}))
	chatActionInfo := buildStructInfo("ChatAction", "internal/types/raw.go", reflect.TypeOf(types.ChatAction{}))
	nestedSectionInfo := buildStructInfo("NestedSection", "internal/types/raw.go", reflect.TypeOf(types.NestedSection{}))
	nestedSectionDataObjectInfo := buildStructInfo("NestedSectionDataObject", "internal/types/raw.go", reflect.TypeOf(types.NestedSectionDataObject{}))
	nestedSectionDataItemInfo := buildStructInfo("NestedSectionDataItem", "internal/types/raw.go", reflect.TypeOf(types.NestedSectionDataItem{}))
	nestedItemDetailInfo := buildStructInfo("NestedItemDetail", "internal/types/raw.go", reflect.TypeOf(types.NestedItemDetail{}))
	nestedActionInfo := buildStructInfo("NestedAction", "internal/types/raw.go", reflect.TypeOf(types.NestedAction{}))
	subAmountInfo := buildStructInfo("SubAmount", "internal/types/event.go", reflect.TypeOf(types.SubAmount{}))
	avatarInfo := buildStructInfo("Avatar", "internal/types/event.go", reflect.TypeOf(types.Avatar{}))
	amountInfo := buildStructInfo("Amount", "internal/types/event.go", reflect.TypeOf(types.Amount{}))
	actionInfo := buildStructInfo("Action", "internal/types/event.go", reflect.TypeOf(types.Action{}))

	// Build the nested action payload tree (reused in multiple places)
	nestedActionChildren := []childMapping{
		{jsonKey: "payload", childInfo: detailActionPayloadInfo, children: detailActionPayloadChildren(
			contextParamsInfo, nestedSectionInfo, nestedSectionDataObjectInfo,
			nestedSectionDataItemInfo, nestedItemDetailInfo, nestedActionInfo,
			displayValueInfo, payloadContentInfo, payloadTrailingInfo, chatActionInfo,
			detailActionPayloadInfo, itemLeadingInfo, iconValueInfo,
		)},
	}

	_ = nestedActionChildren

	// DetailAction children
	detailActionChildren := []childMapping{
		{jsonKey: "payload", childInfo: detailActionPayloadInfo, children: detailActionPayloadChildren(
			contextParamsInfo, nestedSectionInfo, nestedSectionDataObjectInfo,
			nestedSectionDataItemInfo, nestedItemDetailInfo, nestedActionInfo,
			displayValueInfo, payloadContentInfo, payloadTrailingInfo, chatActionInfo,
			detailActionPayloadInfo, itemLeadingInfo, iconValueInfo,
		)},
	}

	// SectionAction children
	sectionActionChildren := []childMapping{
		{jsonKey: "payload", childInfo: actionPayloadInfo, children: []childMapping{
			{jsonKey: "contextParams", childInfo: contextParamsInfo},
			{jsonKey: "action", childInfo: detailActionInfo, children: detailActionChildren},
			{jsonKey: "sections", childInfo: nestedSectionInfo, isArray: true},
		}},
	}

	// ItemDetail children
	itemDetailChildren := []childMapping{
		{jsonKey: "icon", childInfo: iconValueInfo},
		{jsonKey: "displayValue", childInfo: displayValueInfo},
		{jsonKey: "action", childInfo: detailActionInfo, children: detailActionChildren},
		{jsonKey: "content", childInfo: payloadContentInfo},
		{jsonKey: "leading", childInfo: itemLeadingInfo, children: []childMapping{
			{jsonKey: "icon", childInfo: iconValueInfo},
		}},
		{jsonKey: "trailing", childInfo: payloadTrailingInfo},
	}

	// SectionDataItem children
	sectionDataItemChildren := []childMapping{
		{jsonKey: "detail", childInfo: itemDetailInfo, children: itemDetailChildren},
		{jsonKey: "action", childInfo: detailActionInfo, children: detailActionChildren},
	}

	// Section children
	sectionChildren := []childMapping{
		{jsonKey: "actionableTitle", childInfo: actionButtonInfo, children: []childMapping{
			{jsonKey: "action", childInfo: detailActionInfo, children: detailActionChildren},
		}},
		{jsonKey: "button", childInfo: actionButtonInfo, children: []childMapping{
			{jsonKey: "action", childInfo: detailActionInfo, children: detailActionChildren},
		}},
		{jsonKey: "data", childInfo: sectionDataItemInfo, isArray: true, children: sectionDataItemChildren,
			altChildInfo: &headerDataInfo, altChildren: []childMapping{
				{jsonKey: "icon", childInfo: iconValueInfo},
			}},
		{jsonKey: "action", childInfo: sectionActionInfo, children: sectionActionChildren},
		{jsonKey: "steps", childInfo: stepItemInfo, isArray: true, children: []childMapping{
			{jsonKey: "content", childInfo: stepContentInfo},
			{jsonKey: "leading", childInfo: stepLeadingInfo, children: []childMapping{
				{jsonKey: "avatar", childInfo: stepAvatarInfo},
				{jsonKey: "connection", childInfo: stepConnectionInfo},
			}},
		}},
	}

	// TimelineEvent children
	timelineEventChildren := []childMapping{
		{jsonKey: "amount", childInfo: amountInfo},
		{jsonKey: "action", childInfo: actionInfo},
		{jsonKey: "avatar", childInfo: avatarInfo},
		{jsonKey: "subAmount", childInfo: subAmountInfo},
	}

	// TimelineDetails children
	timelineDetailsChildren := []childMapping{
		{jsonKey: "sections", childInfo: sectionInfo, isArray: true, children: sectionChildren},
	}

	return childMapping{
		childInfo: rawEventInfo,
		children: []childMapping{
			{jsonKey: "timelineEvent", childInfo: timelineEventInfo, children: timelineEventChildren},
			{jsonKey: "details", childInfo: timelineDetailsInfo, children: timelineDetailsChildren},
		},
	}
}

// detailActionPayloadChildren builds children for DetailActionPayload (reused in multiple places).
func detailActionPayloadChildren(
	contextParamsInfo, nestedSectionInfo, nestedSectionDataObjectInfo,
	nestedSectionDataItemInfo, nestedItemDetailInfo, nestedActionInfo,
	displayValueInfo, payloadContentInfo, payloadTrailingInfo, chatActionInfo,
	detailActionPayloadInfo, itemLeadingInfo, iconValueInfo structInfo,
) []childMapping {
	return []childMapping{
		{jsonKey: "contextParams", childInfo: contextParamsInfo},
		{jsonKey: "displayValue", childInfo: displayValueInfo},
		{jsonKey: "content", childInfo: payloadContentInfo},
		{jsonKey: "trailing", childInfo: payloadTrailingInfo},
		{jsonKey: "chatAction", childInfo: chatActionInfo, children: []childMapping{
			{jsonKey: "payload", childInfo: detailActionPayloadInfo},
		}},
		{jsonKey: "sections", childInfo: nestedSectionInfo, isArray: true, children: []childMapping{
			{jsonKey: "data", childInfo: nestedSectionDataItemInfo, isArray: true, children: []childMapping{
				{jsonKey: "detail", childInfo: nestedItemDetailInfo, children: []childMapping{
					{jsonKey: "displayValue", childInfo: displayValueInfo},
					{jsonKey: "action", childInfo: nestedActionInfo},
					{jsonKey: "content", childInfo: payloadContentInfo},
					{jsonKey: "leading", childInfo: itemLeadingInfo, children: []childMapping{
						{jsonKey: "icon", childInfo: iconValueInfo},
					}},
					{jsonKey: "trailing", childInfo: payloadTrailingInfo},
				}},
			}},
			{jsonKey: "action", childInfo: nestedActionInfo},
		}},
	}
}

// DetectUnknownFields scans a JSON event file and returns all unknown fields
// by walking the JSON tree against the known Go type hierarchy.
func DetectUnknownFields(jsonData []byte) ([]UnknownField, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(jsonData, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	var fields []UnknownField
	walkObject(raw, typeTree, &fields)
	return fields, nil
}

// walkObject compares JSON keys against a struct's known tags and recurses into children.
func walkObject(raw map[string]json.RawMessage, mapping childMapping, fields *[]UnknownField) {
	info := mapping.childInfo

	// Find unknown fields at this level
	for key, val := range raw {
		if info.jsonTags[key] {
			// Known field - check if it has child mappings to recurse into
			for _, child := range mapping.children {
				if child.jsonKey == key {
					walkRawValue(val, child, fields)
					break
				}
			}
			continue
		}

		// Unknown field
		goType := InferGoType(val)
		*fields = append(*fields, UnknownField{
			StructName:  info.name,
			StructFile:  info.file,
			JSONKey:     key,
			RawValue:    val,
			GoType:      goType,
			GoFieldName: JSONKeyToGoFieldName(key),
		})
	}
}

// walkRawValue handles a raw JSON value that might be an object or array.
func walkRawValue(val json.RawMessage, child childMapping, fields *[]UnknownField) {
	trimmed := trimJSON(val)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return
	}

	if child.isArray {
		if trimmed[0] == '[' {
			// Array of objects
			var items []json.RawMessage
			if err := json.Unmarshal(val, &items); err != nil {
				return
			}
			for _, item := range items {
				walkRawValueObject(item, child, fields)
			}
		} else if trimmed[0] == '{' && child.altChildInfo != nil {
			// Polymorphic: expected array but got object, use alternate mapping
			altMapping := childMapping{
				childInfo: *child.altChildInfo,
				children:  child.altChildren,
			}
			walkRawValueObject(val, altMapping, fields)
		}
	} else {
		walkRawValueObject(val, child, fields)
	}
}

// walkRawValueObject handles a raw JSON value that should be an object.
func walkRawValueObject(val json.RawMessage, child childMapping, fields *[]UnknownField) {
	trimmed := trimJSON(val)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(val, &obj); err != nil {
		return
	}
	walkObject(obj, child, fields)
}

// trimJSON trims whitespace from a raw JSON value.
func trimJSON(raw json.RawMessage) []byte {
	result := make([]byte, len(raw))
	copy(result, raw)
	// Trim leading whitespace
	for len(result) > 0 && (result[0] == ' ' || result[0] == '\t' || result[0] == '\n' || result[0] == '\r') {
		result = result[1:]
	}
	return result
}

// ScanDirectory reads all JSON event files from a directory and detects unknown fields.
func ScanDirectory(dir string) ([]UnknownField, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to list JSON files: %w", err)
	}

	seen := make(map[string]bool) // "StructName:jsonKey" -> already seen
	var allFields []UnknownField

	for _, file := range files {
		if filepath.Base(file) == "metadata.json" {
			continue
		}

		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		fields, err := DetectUnknownFields(data)
		if err != nil {
			continue
		}

		for _, f := range fields {
			key := f.StructName + ":" + f.JSONKey
			if seen[key] {
				continue
			}
			seen[key] = true
			allFields = append(allFields, f)
		}
	}

	return allFields, nil
}

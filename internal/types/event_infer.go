package types

import (
	"errors"
	"fmt"
	"strings"
)

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
	if eventType := inferFromTimeline(te); eventType != "" {
		return eventType, nil
	}
	if eventType, err := inferFromDetailSections(details); err == nil {
		return eventType, nil
	}
	return "", fmt.Errorf("skipped: unknown event type (id='%s', title='%s', subtitle='%s')", te.ID, te.Title, te.Subtitle)
}

func inferFromTimeline(te TimelineEvent) EventType {
	explicitType := te.EventType
	if explicitType == "timeline_legacy_migrated_events" {
		explicitType = ""
	}
	for _, source := range []struct {
		value   string
		mapping map[string]EventType
	}{
		{explicitType, trEventTypeMap},
		{te.Title, titleEventTypeMap},
		{te.Subtitle, subtitleEventTypeMap},
	} {
		if eventType, ok := source.mapping[strings.ToLower(source.value)]; ok {
			return eventType
		}
	}
	for prefix, eventType := range subtitlePrefixesEventTypeMap {
		if strings.HasPrefix(strings.ToLower(te.Subtitle), prefix) {
			return eventType
		}
	}
	return ""
}

// inferFromDetailSections tries to infer event type from detail section patterns
func inferFromDetailSections(details TimelineDetails) (EventType, error) {
	if len(details.Sections) == 0 {
		return "", errors.New("could not find details sections to infer event type")
	}

	for _, section := range details.Sections {
		if eventType := inferFromSection(section); eventType != "" {
			return eventType, nil
		}
	}

	return "", errors.New("could not infer event type from details")
}

func inferFromSection(section Section) EventType {
	title := strings.ToLower(section.Title)
	for prefix, eventType := range sectionTitlePrefixesEventTypeMap {
		if strings.HasPrefix(title, prefix) {
			return eventType
		}
	}
	if title == Overview {
		for _, item := range section.Data {
			if eventType, ok := sectionTitleEventTypeMap[strings.ToLower(item.Title)]; ok {
				return eventType
			}
		}
	}
	return ""
}

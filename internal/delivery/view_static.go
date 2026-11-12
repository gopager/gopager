package delivery

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/gopager/gopager/internal/processing"
)

// static resolves a static catalog field.
func (v *view) static(key string) (processing.Value, bool) {
	if v.withoutEvent && strings.HasPrefix(key, "event.") {
		return processing.Value{}, false
	}
	switch key {
	case "event.type":
		return stringValue(v.event.Type), true
	case "event.occurredAt":
		return timeValue(v.event.OccurredAt), true
	case "event.lateJoin":
		if v.event.Data.LateJoin == nil && v.event.Type != "message.transmission-joined" {
			return processing.Value{}, false
		}
		late := v.event.Data.LateJoin != nil && *v.event.Data.LateJoin
		return boolValue(late), true
	case "event.reason":
		return optionalString(v.event.Data.Reason)
	case "gateway.id", "gateway.name", "gateway.oldState", "gateway.newState", "gateway.contactTime":
		return v.gatewayField(key)
	case "unit.id", "unit.name":
		if len(v.units) != 1 {
			return processing.Value{}, false
		}
		if key == "unit.id" {
			return stringValue(v.units[0].UnitID.String()), true
		}
		return stringValue(v.units[0].Name), true
	}
	if v.message == nil {
		return processing.Value{}, false
	}
	item := v.message.Item
	switch key {
	case "message.id":
		return stringValue(item.MessageID.String()), true
	case "message.time":
		return timeValue(item.MessageTime), true
	case "message.processingStatus":
		return stringValue(item.ProcessingStatus), true
	case "message.correctionStatus":
		return stringValue(item.CorrectionStatus), true
	case "message.protocol":
		if item.Protocol == nil {
			return processing.Value{}, false
		}
		return stringValue(*item.Protocol), true
	case "message.form", "message.text", "message.numeric":
		return payloadField(item.Payload, key)
	case "message.unitIds", "message.unitNames":
		values := make([]processing.Value, 0, len(v.units))
		for _, unit := range v.units {
			if key == "message.unitIds" {
				values = append(values, stringValue(unit.UnitID.String()))
			} else {
				values = append(values, stringValue(unit.Name))
			}
		}
		return listValue(values), true
	}
	return processing.Value{}, false
}

// gatewayField resolves a Gateway field of a Gateway fact.
func (v *view) gatewayField(key string) (processing.Value, bool) {
	if v.gateway == nil {
		return processing.Value{}, false
	}
	switch key {
	case "gateway.id":
		return stringValue(v.gateway.ID.String()), true
	case "gateway.name":
		return stringValue(v.gateway.Name), true
	case "gateway.oldState":
		return optionalString(v.event.Data.OldState)
	case "gateway.newState":
		return optionalString(v.event.Data.NewState)
	case "gateway.contactTime":
		if v.event.Data.ContactTime == nil {
			return processing.Value{}, false
		}
		return timeValue(*v.event.Data.ContactTime), true
	}
	return processing.Value{}, false
}

// payloadField reads form, text and numeric values of a Representative Payload.
func payloadField(payload json.RawMessage, key string) (processing.Value, bool) {
	if len(payload) == 0 {
		return processing.Value{}, false
	}
	var decoded struct {
		Form  string `json:"form"`
		Value string `json:"value"`
	}
	if json.Unmarshal(payload, &decoded) != nil || decoded.Form == "" {
		return processing.Value{}, false
	}
	switch key {
	case "message.form":
		return stringValue(decoded.Form), true
	case "message.text":
		if decoded.Form == "text" {
			return stringValue(decoded.Value), true
		}
	case "message.numeric":
		if decoded.Form == "numeric" {
			return stringValue(decoded.Value), true
		}
	}
	return processing.Value{}, false
}

func optionalString(text string) (processing.Value, bool) {
	if text == "" {
		return processing.Value{}, false
	}
	return stringValue(text), true
}

func stringValue(text string) processing.Value {
	encoded, _ := json.Marshal(text)
	return processing.Value{Kind: processing.StringValue, Value: encoded}
}

func timeValue(instant time.Time) processing.Value {
	encoded, _ := json.Marshal(instant.UTC().Format(time.RFC3339Nano))
	return processing.Value{Kind: processing.TimestampValue, Value: encoded}
}

func boolValue(flag bool) processing.Value {
	encoded, _ := json.Marshal(flag)
	return processing.Value{Kind: processing.BooleanValue, Value: encoded}
}

func listValue(values []processing.Value) processing.Value {
	encoded, _ := json.Marshal(values)
	return processing.Value{Kind: processing.ListValue, Value: encoded}
}

package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

// ResultSchemas types the result namespace: the schema of one output of a Workflow's active
// version. It fails with authz.ErrNotFound for an unknown Workflow or output.
type ResultSchemas interface {
	ResultFieldSchema(ctx context.Context, tx platform.TransactionContext, workflowID uuid.UUID, output string) (processing.FieldSchema, error)
}

// fieldSpec is the declaration and field group of one catalog field.
type fieldSpec struct {
	kind  processing.ValueKind
	list  bool
	group string
}

// staticFields is the fixed part of the field catalog of filters and layouts. Every field is
// optional, because not every event type supplies it. An empty group needs no field permission.
var staticFields = map[string]fieldSpec{
	"event.type":               {kind: processing.StringValue},
	"event.occurredAt":         {kind: processing.TimestampValue},
	"event.lateJoin":           {kind: processing.BooleanValue},
	"event.reason":             {kind: processing.StringValue},
	"message.id":               {kind: processing.StringValue},
	"message.time":             {kind: processing.TimestampValue},
	"message.processingStatus": {kind: processing.StringValue},
	"message.correctionStatus": {kind: processing.StringValue},
	"message.protocol":         {kind: processing.StringValue, group: "payload"},
	"message.form":             {kind: processing.StringValue, group: "payload"},
	"message.text":             {kind: processing.StringValue, group: "payload"},
	"message.numeric":          {kind: processing.StringValue, group: "payload"},
	"message.unitIds":          {kind: processing.StringValue, list: true, group: "unit"},
	"message.unitNames":        {kind: processing.StringValue, list: true, group: "unit"},
	"unit.id":                  {kind: processing.StringValue, group: "unit"},
	"unit.name":                {kind: processing.StringValue, group: "unit"},
	"gateway.id":               {kind: processing.StringValue, group: "gateway"},
	"gateway.name":             {kind: processing.StringValue, group: "gateway"},
	"gateway.oldState":         {kind: processing.StringValue, group: "gateway"},
	"gateway.newState":         {kind: processing.StringValue, group: "gateway"},
	"gateway.contactTime":      {kind: processing.TimestampValue, group: "gateway"},
}

// resultGroup is the field group of every result field. Beyond it, the read side shows a result
// only when the subscriber may read all field groups stored with it.
const resultGroup = "derived"

// staticKey returns the catalog key of a two-segment reference, or "".
func staticKey(ref processing.FieldRef) string {
	if len(ref.Path) != 1 {
		return ""
	}
	return ref.Namespace + "." + ref.Path[0]
}

// resultRef returns the Workflow and output of a result reference.
func resultRef(ref processing.FieldRef) (uuid.UUID, string, bool) {
	if ref.Namespace != "result" || len(ref.Path) != 2 || ref.Path[1] == "" {
		return uuid.Nil, "", false
	}
	workflow, err := uuid.Parse(ref.Path[0])
	if err != nil || workflow == uuid.Nil || workflow.String() != ref.Path[0] {
		return uuid.Nil, "", false
	}
	return workflow, ref.Path[1], true
}

// schemaOf builds the declared schema of a static field.
func (s fieldSpec) schema() processing.FieldSchema {
	if s.list {
		element := processing.FieldSchema{Kind: s.kind}
		return processing.FieldSchema{Kind: processing.ListValue, Optional: true, Element: &element}
	}
	return processing.FieldSchema{Kind: s.kind, Optional: true}
}

// fieldSet declares the catalog for one compilation and remembers the schemas it handed out,
// so that evaluation can treat a result value of another kind as absent. It is safe for
// concurrent use.
type fieldSet struct {
	ctx     context.Context
	tx      platform.TransactionContext
	results ResultSchemas

	mu      sync.Mutex
	schemas map[string]processing.FieldSchema
}

// newFieldSet declares the catalog; result fields are typed through results inside tx.
func newFieldSet(ctx context.Context, tx platform.TransactionContext, results ResultSchemas) *fieldSet {
	return &fieldSet{ctx: ctx, tx: tx, results: results, schemas: map[string]processing.FieldSchema{}}
}

func refKey(ref processing.FieldRef) string {
	return ref.Namespace + "\x00" + strings.Join(ref.Path, "\x00")
}

// FieldSchema implements processing.Declarations.
func (f *fieldSet) FieldSchema(ref processing.FieldRef) (processing.FieldSchema, error) {
	if spec, ok := staticFields[staticKey(ref)]; ok {
		return spec.schema(), nil
	}
	workflow, output, ok := resultRef(ref)
	if !ok {
		return processing.FieldSchema{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("unknown Delivery field"))
	}
	f.mu.Lock()
	cached, known := f.schemas[refKey(ref)]
	f.mu.Unlock()
	if known {
		return cached, nil
	}
	schema, err := f.results.ResultFieldSchema(f.ctx, f.tx, workflow, output)
	if errors.Is(err, authz.ErrNotFound) {
		return processing.FieldSchema{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("unknown result field"))
	}
	if err != nil {
		return processing.FieldSchema{}, err
	}
	schema.Optional = true
	f.mu.Lock()
	f.schemas[refKey(ref)] = schema
	f.mu.Unlock()
	return schema, nil
}

// schemaFor returns the schema handed out for a reference.
func (f *fieldSet) schemaFor(ref processing.FieldRef) (processing.FieldSchema, bool) {
	if spec, ok := staticFields[staticKey(ref)]; ok {
		return spec.schema(), true
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	schema, ok := f.schemas[refKey(ref)]
	return schema, ok
}

// fieldGroups returns the field groups the references need, without duplicates.
func fieldGroups(refs []processing.FieldRef) []string {
	seen := map[string]bool{}
	var groups []string
	add := func(group string) {
		if group != "" && !seen[group] {
			seen[group] = true
			groups = append(groups, group)
		}
	}
	for _, ref := range refs {
		if spec, ok := staticFields[staticKey(ref)]; ok {
			add(spec.group)
			continue
		}
		if _, _, ok := resultRef(ref); ok {
			add(resultGroup)
		}
	}
	return groups
}

// GatewayView is the part of a Gateway a Delivery may use, with the Agencies its Networks
// belong to for authorization.
type GatewayView struct {
	ID        uuid.UUID
	Name      string
	AgencyIDs []uuid.UUID
}

// eventView is the classified part of a fact a Delivery may use.
type eventView struct {
	ID            uuid.UUID
	Type          string
	OccurredAt    time.Time
	RecordedAt    time.Time
	ReplayMode    string
	CorrelationID *uuid.UUID
	CausationID   *uuid.UUID
	Data          eventData
}

// eventData holds the data fields Delivery reads from the deliverable fact types.
type eventData struct {
	MessageID      uuid.UUID  `json:"messageId"`
	TransmissionID uuid.UUID  `json:"transmissionId"`
	ResultID       uuid.UUID  `json:"resultId"`
	GatewayID      uuid.UUID  `json:"gatewayId"`
	OldState       string     `json:"oldState"`
	NewState       string     `json:"newState"`
	ContactTime    *time.Time `json:"contactTime"`
	LateJoin       *bool      `json:"lateJoin"`
	Reason         string     `json:"reason"`
}

// parseEvent decodes the envelope fields Delivery needs from a stored record.
func parseEvent(id uuid.UUID, envelope json.RawMessage) (eventView, error) {
	var decoded struct {
		Type          string     `json:"type"`
		OccurredAt    time.Time  `json:"occurredAt"`
		RecordedAt    time.Time  `json:"recordedAt"`
		ReplayMode    string     `json:"replayMode"`
		CorrelationID *uuid.UUID `json:"correlationId"`
		CausationID   *uuid.UUID `json:"causationId"`
		Data          eventData  `json:"data"`
	}
	if err := json.Unmarshal(envelope, &decoded); err != nil {
		return eventView{}, platform.NewFailure(platform.CodeInternalError, err)
	}
	return eventView{ID: id, Type: decoded.Type, OccurredAt: decoded.OccurredAt.UTC(), RecordedAt: decoded.RecordedAt.UTC(),
		ReplayMode: decoded.ReplayMode, CorrelationID: decoded.CorrelationID, CausationID: decoded.CausationID,
		Data: decoded.Data}, nil
}

// view is everything one Delivery's filter and layout can read: the fact, the authorized
// Message or Gateway, the Delivery's relevant Units and the subscriber's readable groups.
type view struct {
	event   eventView
	message *readmodel.DeliveryProjection
	units   []readmodel.UnitRecord
	gateway *GatewayView
	groups  map[string]bool
	fields  *fieldSet
}

// LookupField implements processing.FieldReader. A field whose group the subscriber cannot read
// now is absent, whatever was checked when the Subscription was saved.
func (v *view) LookupField(_ context.Context, ref processing.FieldRef) (processing.FieldValue, error) {
	absent := processing.FieldValue{State: processing.FieldAbsent}
	if spec, ok := staticFields[staticKey(ref)]; ok {
		if spec.group != "" && !v.groups[spec.group] {
			return absent, nil
		}
		value, ok := v.static(staticKey(ref))
		if !ok {
			return absent, nil
		}
		return processing.FieldValue{State: processing.FieldPresent, Value: &value}, nil
	}
	workflow, output, ok := resultRef(ref)
	if !ok || !v.groups[resultGroup] || v.message == nil || v.message.Item.Derived == nil {
		return absent, nil
	}
	schema, ok := v.fields.schemaFor(ref)
	if !ok {
		return absent, nil
	}
	for _, item := range *v.message.Item.Derived {
		if item.WorkflowID != workflow {
			continue
		}
		field, ok := item.Fields[output]
		if !ok || field.State != string(processing.FieldPresent) || len(field.Value) == 0 {
			return absent, nil
		}
		var value processing.Value
		if json.Unmarshal(field.Value, &value) != nil {
			return absent, nil
		}
		if _, err := value.Native(schema); err != nil {
			return absent, nil
		}
		return processing.FieldValue{State: processing.FieldPresent, Value: &value}, nil
	}
	return absent, nil
}

// static resolves a static catalog field.
func (v *view) static(key string) (processing.Value, bool) {
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

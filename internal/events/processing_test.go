package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

func completionFixture(t *testing.T) ProcessingCompletionFact {
	t.Helper()
	value, err := processing.Literal(processing.StringValue, "classified result")
	if err != nil {
		t.Fatal(err)
	}
	transmission := uuid.New()
	return ProcessingCompletionFact{ID: uuid.New(), TransmissionID: transmission, MessageID: uuid.New(),
		MessageTime: time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC),
		WorkflowID:  uuid.New(), DefinitionVersionID: uuid.New(), ResultID: uuid.New(),
		ResultRevision: 3, SourceRevision: 7, ContextRevision: 9, OccurredAt: time.Unix(100, 0),
		Outputs: outputDeclarations(processing.StringValue),
		Result: processing.Evaluation{Selected: true, Fields: map[string]map[string]processing.FieldValue{"alerts": {"level": {
			State: processing.FieldPresent, Value: &value, Dependencies: []processing.Dependency{{
				TransmissionID: transmission, NetworkID: uuid.New(), SourceRevision: 7, ContextRevision: 9,
				Groups: []string{"payload"},
			}},
		}}}}}
}

// outputDeclarations declares the Attribute field alerts.level with the given kind.
func outputDeclarations(kind processing.ValueKind) processing.Declarations {
	provider := fields.NewStaticProvider(1, []fields.Field{{
		Ref:    processing.FieldRef{Namespace: "attributes", Path: []string{"alerts", "level"}},
		Type:   processing.FieldSchema{Kind: kind},
		Group:  "derived",
		Scopes: []fields.Scope{fields.ScopeMessage},
	}})
	return fields.New(provider).Declarations(fields.ScopeMessage, false)
}

func TestProcessingFactFactoriesClosedVocabularyAndDetachedEnvelope(t *testing.T) {
	t.Parallel()
	now := time.Unix(101, 0)
	causation := uuid.New()
	fact := ProcessingConfigurationFact{ID: uuid.New(), WorkflowID: uuid.New(), Revision: 1,
		OccurredAt: time.Unix(100, 0), CausationID: &causation, ChangeKind: "version-saved"}
	envelope, err := fact.Envelope(now)
	if err != nil {
		t.Fatal(err)
	}
	before := *envelope.CausationID
	causation = uuid.New()
	if *envelope.CausationID != before || envelope.Producer.Domain != "processing" ||
		envelope.Type != "configuration.changed" || string(envelope.Data) != `{"changeKind":"version-saved"}` {
		t.Fatalf("configuration escaped producer boundary: %+v", envelope)
	}
	for _, kind := range []string{"password-changed", "", "membership-changed"} {
		fact.ChangeKind = kind
		if _, err := fact.Envelope(now); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Fatalf("older/unknown change entered Processing: %q, %v", kind, err)
		}
	}
	completion := completionFixture(t)
	completion.Historical = true
	published, err := completion.Envelope(now)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(published)
	if err != nil {
		t.Fatal(err)
	}
	if published.AggregateRef.ID != completion.ResultID || published.AggregateRef.Type != "processingResult" ||
		published.AggregateRevision != completion.ResultRevision || published.ReplayMode != ReplayReprocess ||
		published.Type != "processing.completed" || !strings.Contains(string(encoded), `"groups":["payload"]`) ||
		!strings.Contains(string(encoded), `"sourceRevision":"7"`) || !strings.Contains(string(encoded), `"status":"completed"`) {
		t.Fatalf("publication lost source/replay/aggregate identity: %s", encoded)
	}
}

func TestProcessingCompletionRejectsPartialHiddenUncorrelatedAndUnpublishedResults(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*ProcessingCompletionFact){
		"missing message": func(f *ProcessingCompletionFact) { f.MessageID = uuid.Nil },
		"missing schema":  func(f *ProcessingCompletionFact) { f.Outputs = nil },
		"not selected":    func(f *ProcessingCompletionFact) { f.Result.Selected = false },
		"empty missing field": func(f *ProcessingCompletionFact) {
			f.Result.Fields = map[string]map[string]processing.FieldValue{"alerts": {}}
		},
		"hidden": func(f *ProcessingCompletionFact) {
			field := f.Result.Fields["alerts"]["level"]
			field.State, field.Value = processing.FieldState("hidden"), nil
			f.Result.Fields["alerts"]["level"] = field
		},
		"wrong type": func(f *ProcessingCompletionFact) {
			f.Outputs = outputDeclarations(processing.BooleanValue)
		},
		"wrong revision": func(f *ProcessingCompletionFact) { f.Result.Fields["alerts"]["level"].Dependencies[0].SourceRevision++ },
		"mixed containers": func(f *ProcessingCompletionFact) {
			f.Result.TargetResults = []processing.TargetResult{{TargetPosition: 0, Fields: f.Result.Fields}}
		},
		"duplicate target": func(f *ProcessingCompletionFact) {
			fields := f.Result.Fields
			f.Result.Fields = nil
			f.Result.TargetResults = []processing.TargetResult{{TargetPosition: 0, Fields: fields}, {TargetPosition: 0, Fields: fields}}
		},
		"out of order": func(f *ProcessingCompletionFact) {
			fields := f.Result.Fields
			f.Result.Fields = nil
			f.Result.TargetResults = []processing.TargetResult{{TargetPosition: 1, Fields: fields}, {TargetPosition: 0, Fields: fields}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fact := completionFixture(t)
			mutate(&fact)
			if _, err := fact.Envelope(time.Unix(101, 0)); platform.FailureCode(err) != platform.CodeInvalidInput {
				t.Fatalf("invalid completion became accepted: %v", err)
			}
		})
	}
	fact := completionFixture(t)
	fields := fact.Result.Fields
	fact.Result.Fields = nil
	fact.Result.TargetResults = []processing.TargetResult{{TargetPosition: 0, Fields: fields}, {TargetPosition: 2, Fields: fields}}
	if _, err := fact.Envelope(time.Unix(101, 0)); err != nil {
		t.Fatalf("ordered selected original targets were rejected: %v", err)
	}
}

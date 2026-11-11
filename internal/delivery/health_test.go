package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

type fixedRegistry struct{ registry *fields.Registry }

func (r fixedRegistry) Registry(context.Context, platform.TransactionContext) (*fields.Registry, error) {
	return r.registry, nil
}

type fixedLayouts struct {
	layout processing.Layout
	err    error
}

func (l fixedLayouts) ResolveLayout(context.Context, platform.TransactionContext, processing.LayoutRequest) (processing.ResolvedLayout, error) {
	return processing.ResolvedLayout{ID: uuid.New(), Version: 1, Layout: l.layout}, l.err
}

func (fixedLayouts) CheckUse(context.Context, platform.TransactionContext, authz.Authority, uuid.UUID, time.Time) error {
	return nil
}

func healthDeps(layouts fixedLayouts) Dependencies {
	schema := attributes.Schema{Key: "incident", Subject: attributes.SubjectMessage, Fields: map[string]attributes.Field{
		"level": {Kind: processing.IntegerValue},
		"old":   {Kind: processing.IntegerValue, Deprecated: true},
	}}
	registry := fields.New(fields.Builtin(), attributes.NewProvider([]attributes.Schema{schema}, 1))
	return Dependencies{Fields: fixedRegistry{registry}, Layouts: layouts}
}

func compare(path ...string) processing.Predicate {
	operand := processing.Value{Kind: processing.IntegerValue, Value: []byte(`"1"`)}
	return processing.Predicate{Op: "eq", Field: &processing.FieldRef{Namespace: "attributes", Path: path}, Value: &operand}
}

func fieldBlock(namespace string, path ...string) processing.LayoutBlock {
	return processing.LayoutBlock{Kind: "field", Field: &processing.FieldRef{Namespace: namespace, Path: path}}
}

func TestHealthCheckClassifiesWhatDoesNotCompile(t *testing.T) {
	t.Parallel()
	deep := compare("incident", "level")
	for range processing.MaxDepth + 1 {
		deep = processing.Predicate{Op: "not", Args: []processing.Predicate{deep}}
	}
	badOperand := compare("incident", "level")
	badOperand.Value = &processing.Value{Kind: processing.StringValue, Value: []byte(`"x"`)}
	unknown, deprecated := compare("incident", "nothing"), compare("incident", "old")
	for name, test := range map[string]struct {
		filter *processing.Predicate
		layout processing.Layout
		reason string
	}{
		"valid":                  {filter: &[]processing.Predicate{compare("incident", "level")}[0]},
		"unknown field":          {filter: &unknown, reason: ReasonFieldUnavailable},
		"deprecated field":       {filter: &deprecated, reason: ReasonFieldUnavailable},
		"unknown below a not":    {filter: &processing.Predicate{Op: "not", Args: []processing.Predicate{unknown}}, reason: ReasonFieldUnavailable},
		"operand of wrong kind":  {filter: &badOperand, reason: ReasonFilterInvalid},
		"nesting bound":          {filter: &deep, reason: ReasonResourceLimit},
		"layout with unknown":    {layout: processing.Layout{SchemaVersion: 1, OutputKind: "text", Blocks: []processing.LayoutBlock{fieldBlock("attributes", "incident", "nothing")}}, reason: ReasonFieldUnavailable},
		"layout with deprecated": {layout: processing.Layout{SchemaVersion: 1, OutputKind: "text", Blocks: []processing.LayoutBlock{fieldBlock("attributes", "incident", "old")}}, reason: ReasonFieldUnavailable},
		"malformed layout":       {layout: processing.Layout{SchemaVersion: 1, OutputKind: "text", Blocks: []processing.LayoutBlock{{Kind: "nothing"}}}, reason: ReasonLayoutInvalid},
		"valid layout":           {layout: processing.Layout{SchemaVersion: 1, OutputKind: "text", Blocks: []processing.LayoutBlock{fieldBlock("attributes", "incident", "level")}}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			subscription := Subscription{Filter: test.filter}
			if test.layout.OutputKind != "" {
				id := uuid.New()
				subscription.LayoutID = &id
			}
			_, reason, err := healthCheck(context.Background(), nil, healthDeps(fixedLayouts{layout: test.layout}), subscription,
				map[string]bool{"derived": true})
			if err != nil || reason != test.reason {
				t.Fatalf("reason = %q, %v; want %q", reason, err, test.reason)
			}
		})
	}
}

func TestHealthCheckRequiresTheReadableGroupsOfFilterLayoutAndSplit(t *testing.T) {
	t.Parallel()
	layout := processing.Layout{SchemaVersion: 1, OutputKind: "text", Blocks: []processing.LayoutBlock{fieldBlock("message", "text")}}
	id := uuid.New()
	level := compare("incident", "level")
	for name, test := range map[string]struct {
		subscription Subscription
		readable     map[string]bool
		reason       string
	}{
		"filter group missing": {Subscription{Filter: &level}, map[string]bool{"payload": true}, ReasonFieldUnavailable},
		"filter group present": {Subscription{Filter: &level}, map[string]bool{"derived": true}, ""},
		"layout group missing": {Subscription{LayoutID: &id}, map[string]bool{"derived": true}, ReasonFieldUnavailable},
		"split needs unit":     {Subscription{SplitByUnit: true}, map[string]bool{"derived": true}, ReasonFieldUnavailable},
		"nothing needed":       {Subscription{}, nil, ""},
	} {
		_, reason, err := healthCheck(context.Background(), nil, healthDeps(fixedLayouts{layout: layout}), test.subscription, test.readable)
		if err != nil || reason != test.reason {
			t.Errorf("%s: reason = %q, %v; want %q", name, reason, err, test.reason)
		}
	}
}

func TestHealthCheckIgnoresAMissingLayout(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	_, reason, err := healthCheck(context.Background(), nil, healthDeps(fixedLayouts{err: authz.ErrNotFound}),
		Subscription{LayoutID: &id}, nil)
	if err != nil || reason != "" {
		t.Fatalf("reason = %q, %v; rendering reports a missing Layout per Delivery", reason, err)
	}
	broken := errors.New("storage down")
	if _, _, err := healthCheck(context.Background(), nil, healthDeps(fixedLayouts{err: broken}), Subscription{LayoutID: &id}, nil); !errors.Is(err, broken) {
		t.Fatalf("an infrastructure error must not become a health reason, got %v", err)
	}
}

// LockCatalog implements FieldRegistry; the fake has no catalog.
func (r fixedRegistry) LockCatalog(context.Context, platform.TransactionContext) error { return nil }

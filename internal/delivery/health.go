package delivery

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// FieldRegistry supplies the field registry visible in a transaction. The registry is the one
// place that declares which fields filters and Layouts may read.
type FieldRegistry interface {
	Registry(ctx context.Context, tx platform.TransactionContext) (*fields.Registry, error)
}

// Layouts resolves Layouts for rendering and decides whether an authority may choose one.
type Layouts interface {
	processing.LayoutResolver
	// CheckUse fails with authz.ErrInvalidInput when the Layout is missing or the authority may
	// not use it.
	CheckUse(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, layoutID uuid.UUID,
		now time.Time) error
}

// selection is the part of a Subscription that reads fields.
type selection struct {
	filter      *processing.Predicate
	layoutID    *uuid.UUID
	splitByUnit bool
}

// compiled is a selection checked against one registry snapshot.
type compiled struct {
	declarations fields.Declarations
	filter       *fields.Filter
	// layoutKind is the output kind of the chosen Layout; empty when none is chosen or it is gone.
	layoutKind string
	// groups are the field groups the filter, the chosen Layout and the split read.
	groups []string
}

// fault is a selection that does not compile, with the health reason it stands for.
type fault struct {
	reason string
	err    error
}

func (f *fault) Error() string { return f.err.Error() }
func (f *fault) Unwrap() error { return f.err }

// trackedDeclarations remembers whether a field was refused, so that a Layout that fails to
// compile can be told apart: an unavailable field versus a malformed Layout.
type trackedDeclarations struct {
	inner       fields.Declarations
	unavailable bool
}

// FieldSchema implements processing.Declarations.
func (t *trackedDeclarations) FieldSchema(ref processing.FieldRef) (processing.FieldSchema, error) {
	schema, err := t.inner.FieldSchema(ref)
	if err != nil {
		t.unavailable = true
	}
	return schema, err
}

// compileSelection checks the filter and the chosen Layout against the current registry, strict.
// A selection that does not compile is a *fault; any other error is infrastructure. A Layout
// that cannot be resolved is not a fault here: rendering reports it per Delivery.
func compileSelection(ctx context.Context, tx platform.TransactionContext, deps Dependencies, chosen selection) (*compiled, error) {
	registry, err := deps.Fields.Registry(ctx, tx)
	if err != nil {
		return nil, err
	}
	result := &compiled{declarations: registry.Declarations(fields.ScopeDelivery, true)}
	groups := map[string]bool{}
	if chosen.filter != nil {
		if result.filter, err = fields.CompileFilter(*chosen.filter, result.declarations); err != nil {
			return nil, &fault{reason: filterReason(err, *chosen.filter, result.declarations), err: err}
		}
		for _, group := range result.filter.Groups() {
			groups[group] = true
		}
	}
	if chosen.layoutID != nil {
		resolved, err := deps.Layouts.ResolveLayout(ctx, tx, processing.LayoutRequest{LayoutID: chosen.layoutID})
		switch {
		case errors.Is(err, authz.ErrNotFound) || errors.Is(err, authz.ErrInvalidInput):
		case err != nil:
			return nil, err
		default:
			tracked := &trackedDeclarations{inner: result.declarations}
			program, err := processing.CompileLayout(ctx, resolved.Layout, tracked)
			if err != nil {
				return nil, layoutFault(ctx, err, tracked.unavailable)
			}
			for _, ref := range program.Fields() {
				field, err := result.declarations.Field(ref)
				if err != nil {
					return nil, &fault{reason: ReasonFieldUnavailable, err: err}
				}
				groups[field.Group] = true
			}
			result.layoutKind = resolved.Layout.OutputKind
		}
	}
	if chosen.splitByUnit {
		groups["unit"] = true
	}
	delete(groups, "")
	for group := range groups {
		result.groups = append(result.groups, group)
	}
	slices.Sort(result.groups)
	return result, nil
}

// filterReason classifies a filter that does not compile.
func filterReason(err error, filter processing.Predicate, declarations fields.Declarations) string {
	switch {
	case platform.FailureCode(err) == platform.CodeResourceLimit:
		return ReasonResourceLimit
	case refusesField(filter, declarations, 1):
		return ReasonFieldUnavailable
	}
	return ReasonFilterInvalid
}

// refusesField reports whether the predicate names a field the registry refuses here: unknown,
// outside the scope or deprecated.
func refusesField(predicate processing.Predicate, declarations fields.Declarations, depth int) bool {
	if depth > processing.MaxDepth {
		return false
	}
	if predicate.Field != nil {
		_, err := declarations.Field(*predicate.Field)
		return err != nil
	}
	for _, arg := range predicate.Args {
		if refusesField(arg, declarations, depth+1) {
			return true
		}
	}
	return false
}

// layoutFault classifies a Layout that does not compile. A cancelled context is no fault.
func layoutFault(ctx context.Context, err error, unavailable bool) error {
	switch {
	case ctx.Err() != nil:
		return err
	case platform.FailureCode(err) == platform.CodeResourceLimit:
		return &fault{reason: ReasonResourceLimit, err: err}
	case unavailable:
		return &fault{reason: ReasonFieldUnavailable, err: err}
	case platform.FailureCode(err) == platform.CodeInvalidInput:
		return &fault{reason: ReasonLayoutInvalid, err: err}
	}
	return err
}

// unreadable reports whether the subscriber cannot read all groups the selection needs.
func (c *compiled) unreadable(readable map[string]bool) bool {
	return slices.ContainsFunc(c.groups, func(group string) bool { return !readable[group] })
}

// healthCheck compiles a Subscription's selection against the current registry and requires the
// subscriber's readable groups to cover it. It returns the compiled selection, or the reason
// the Subscription is invalid.
func healthCheck(ctx context.Context, tx platform.TransactionContext, deps Dependencies, subscription Subscription,
	readable map[string]bool) (*compiled, string, error) {
	result, err := compileSelection(ctx, tx, deps, selection{filter: subscription.Filter, layoutID: subscription.LayoutID,
		splitByUnit: subscription.SplitByUnit})
	var failure *fault
	if errors.As(err, &failure) {
		return nil, failure.reason, nil
	}
	if err != nil {
		return nil, "", err
	}
	if result.unreadable(readable) {
		return nil, ReasonFieldUnavailable, nil
	}
	return result, "", nil
}

// markInvalid records a Subscription as invalid with its reason and appends the health fact when
// that changed something. It never changes the revision.
func markInvalid(ctx context.Context, tx platform.TransactionContext, deps Dependencies, subscription Subscription,
	reason string, now time.Time) error {
	changed, err := deps.Store.SetSubscriptionHealth(ctx, tx, subscription.ID, HealthInvalid, reason, now)
	if err != nil || !changed {
		return err
	}
	return appendHealthFact(ctx, tx, deps, subscription, HealthInvalid, reason, now)
}

// appendHealthFact appends the non-deliverable subscription.health-changed fact.
func appendHealthFact(ctx context.Context, tx platform.TransactionContext, deps Dependencies, subscription Subscription,
	health, reason string, now time.Time) error {
	_, err := deps.Facts.AppendSubscriptionHealthFact(ctx, tx, events.SubscriptionHealthFact{ID: uuid.New(),
		Subscription: subscription.ID, Revision: subscription.Revision, OccurredAt: now.Truncate(time.Microsecond),
		Health: health, Reason: reason})
	return err
}

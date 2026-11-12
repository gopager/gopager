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
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

// Projector is the Query read side as Delivery uses it: the Message under a resolved authority,
// in the caller's transaction. *readmodel.Service implements it.
type Projector interface {
	ProjectForDelivery(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, now time.Time,
		messageID, resultID uuid.UUID) (readmodel.DeliveryProjection, bool, error)
}

// Dependencies are the ports the Delivery services share.
type Dependencies struct {
	Store     Store
	Instances Instances
	Targets   Targets
	Layouts   Layouts
	Projector Projector
	Fields    FieldRegistry
	Facts     events.SubscriptionAppender
	Authority PrincipalAuthority
}

// validate fails with authz.ErrInvalidInput when a port is missing.
func (d Dependencies) validate() error {
	if d.Store == nil || d.Instances == nil || d.Targets == nil || d.Layouts == nil || d.Projector == nil ||
		d.Fields == nil || d.Facts == nil || d.Authority == nil {
		return authz.ErrInvalidInput
	}
	return nil
}

// split is one Delivery's share of a fact: its split key, relevant Units, Agencies and age anchor.
type split struct {
	key      string
	units    []readmodel.UnitRecord
	agencies []uuid.UUID
	anchor   time.Time
}

// subject is a fact as one subscriber may see it now.
type subject struct {
	authority authz.Authority
	groups    map[string]bool
	event     eventView
	message   *readmodel.DeliveryProjection
	gateway   *GatewayView
	splits    []split
	// memo shares the projection among the Subscriptions of one subscriber within a fan-out
	// event; nil outside fan-out.
	memo *subjectMemo
}

// absentAuthority reports a failure that means the subscriber cannot act now (suspended,
// deleted or without usable grants), as opposed to an infrastructure failure.
func absentAuthority(err error) bool {
	switch platform.FailureCode(err) {
	case platform.CodeLoginRequired, platform.CodeAuthorityUnavailable, platform.CodeForbidden, platform.CodeNotFound:
		return true
	}
	return errors.Is(err, identity.ErrLoginRequired) || errors.Is(err, authz.ErrNotFound) || errors.Is(err, authz.ErrForbidden)
}

// observe resolves the subscriber and the authorized projection of the fact. It returns nil
// without error when the subscriber has no current authority or may not see the fact's data.
func observe(ctx context.Context, tx platform.TransactionContext, deps Dependencies, now time.Time,
	subscription Subscription, event eventView) (*subject, error) {
	current, err := resolveSubject(ctx, tx, deps, event, subscription.SubscriberID)
	if err != nil || current == nil {
		return nil, err
	}
	return current.project(ctx, tx, deps, now, subscription)
}

// resolveSubject resolves the subscriber's current authority and readable field groups. It
// returns nil without error when the subscriber cannot act now.
func resolveSubject(ctx context.Context, tx platform.TransactionContext, deps Dependencies, event eventView,
	subscriber uuid.UUID) (*subject, error) {
	authority, err := deps.Authority.ResolvePrincipalAuthority(ctx, tx, subscriber)
	if err != nil {
		if absentAuthority(err) {
			return nil, nil
		}
		return nil, err
	}
	readable, err := authz.ReadableFieldGroups(authority)
	if err != nil {
		if absentAuthority(err) {
			return nil, nil
		}
		return nil, err
	}
	current := &subject{authority: authority, groups: map[string]bool{}, event: event}
	for _, group := range readable {
		current.groups[group] = true
	}
	return current, nil
}

// project reads the authorized data of the fact: the Message with its splits or the Gateway. It
// returns nil without error when the subscriber may not see the data.
func (s *subject) project(ctx context.Context, tx platform.TransactionContext, deps Dependencies, now time.Time,
	subscription Subscription) (*subject, error) {
	entry, _ := events.LookupType(s.event.Type)
	switch entry.Projection {
	case events.ProjectionMessage:
		return s.observeMessage(ctx, tx, deps, now, subscription)
	case events.ProjectionGateway:
		return s.observeGateway(ctx, tx, deps, now)
	}
	return nil, nil
}

// messageProjection returns the fact's Message as the subscriber's authority may read it, or nil
// without error when it is gone, not eligible or, for processing.completed, lacks the fact's
// result. Within a fan-out event the answer is computed once per subscriber and shared by that
// subscriber's Subscriptions; the start check projects once per start and shares nothing.
func (s *subject) messageProjection(ctx context.Context, tx platform.TransactionContext, deps Dependencies,
	now time.Time) (*readmodel.DeliveryProjection, error) {
	if s.memo != nil && s.memo.projected {
		return s.memo.projection, nil
	}
	projection, err := s.projectMessage(ctx, tx, deps, now)
	if err != nil {
		return nil, err
	}
	if s.memo != nil {
		s.memo.projected, s.memo.projection = true, projection
	}
	return projection, nil
}

// projectMessage reads the projection of the fact's Message under the subscriber's authority.
func (s *subject) projectMessage(ctx context.Context, tx platform.TransactionContext, deps Dependencies,
	now time.Time) (*readmodel.DeliveryProjection, error) {
	message, err := deps.Store.CurrentMessage(ctx, tx, s.event.Data.MessageID, s.event.Data.TransmissionID)
	if errors.Is(err, authz.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	projection, ok, err := deps.Projector.ProjectForDelivery(ctx, tx, s.authority, now, message, s.event.Data.ResultID)
	if err != nil {
		if absentAuthority(err) {
			return nil, nil
		}
		return nil, err
	}
	if !ok || s.event.Type == "processing.completed" && (s.event.Data.ResultID == uuid.Nil || !projection.ResultVisible) {
		return nil, nil
	}
	return &projection, nil
}

// observeMessage projects the fact's Message and builds the splits.
func (s *subject) observeMessage(ctx context.Context, tx platform.TransactionContext, deps Dependencies, now time.Time,
	subscription Subscription) (*subject, error) {
	if s.event.Data.MessageID == uuid.Nil {
		return nil, nil
	}
	projection, err := s.messageProjection(ctx, tx, deps, now)
	if err != nil || projection == nil {
		return nil, err
	}
	s.message = projection
	withoutUnit := split{key: "", anchor: projection.Item.MessageTime}
	for i, appearance := range projection.WithoutUnit {
		withoutUnit.agencies = appendUnique(withoutUnit.agencies, appearance.AgencyID)
		if i == 0 || appearance.FirstObservedAt.Before(withoutUnit.anchor) {
			withoutUnit.anchor = appearance.FirstObservedAt.UTC()
		}
	}
	if !subscription.SplitByUnit || len(projection.Units) == 0 {
		all := withoutUnit
		all.units = slices.Clone(projection.Units)
		for i, unit := range projection.Units {
			all.agencies = appendUnique(all.agencies, unit.AgencyID)
			if (i == 0 && len(projection.WithoutUnit) == 0) || unit.FirstObservedAt.Before(all.anchor) {
				all.anchor = unit.FirstObservedAt.UTC()
			}
		}
		s.splits = []split{all}
		return s, nil
	}
	for _, unit := range projection.Units {
		s.splits = append(s.splits, split{key: unit.UnitID.String(), units: []readmodel.UnitRecord{unit},
			agencies: []uuid.UUID{unit.AgencyID}, anchor: unit.FirstObservedAt.UTC()})
	}
	return s, nil
}

// observeGateway authorizes a Gateway fact through gateway.read on one of its Agencies.
func (s *subject) observeGateway(ctx context.Context, tx platform.TransactionContext, deps Dependencies, now time.Time) (*subject, error) {
	gateway, err := deps.Store.Gateway(ctx, tx, s.event.Data.GatewayID)
	if errors.Is(err, authz.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	reach, err := authz.ReachableAgencies(s.authority, "gateway.read", now)
	if err != nil || !slices.ContainsFunc(gateway.AgencyIDs, reach.Contains) {
		return nil, nil
	}
	s.gateway = &gateway
	s.splits = []split{{key: "", agencies: slices.Clone(gateway.AgencyIDs), anchor: s.event.OccurredAt}}
	return s, nil
}

func appendUnique(ids []uuid.UUID, id uuid.UUID) []uuid.UUID {
	if slices.Contains(ids, id) {
		return ids
	}
	return append(ids, id)
}

// view returns the field view of one split.
func (s *subject) view(part split, declarations fields.Declarations) *view {
	return &view{event: s.event, message: s.message, units: part.units, gateway: s.gateway, groups: s.groups,
		declarations: declarations, withoutUnit: part.key == "" && s.message != nil && len(s.message.WithoutUnit) > 0}
}

// rendering is the input of one Delivery or the failure code that replaces it.
type rendering struct {
	input            *DeliveryInput
	code             string
	layoutID         *uuid.UUID
	layoutVersion    int64
	instanceRevision int64
	descriptor       TargetDescriptor
	available        bool
	// denied reports that the subscriber has no target use for the instance. A denied target is
	// never a pending dependency: no Delivery is created for it.
	denied bool
}

// render resolves the current target, the layout and the inputs for one split. An unavailable
// target yields available=false; materialization failures yield a code. A subscriber without
// target use for the instance yields denied=true, so a revoked use stops every future send.
func render(ctx context.Context, tx platform.TransactionContext, deps Dependencies, subscription Subscription,
	target TargetReference, current *view) (rendering, error) {
	instance, err := deps.Instances.CurrentInstance(ctx, tx, target.InstanceID)
	if errors.Is(err, authz.ErrNotFound) {
		return rendering{}, nil
	}
	if err != nil {
		return rendering{}, err
	}
	if !instance.Enabled {
		return rendering{instanceRevision: instance.Revision}, nil
	}
	if !instance.AllowsTargetUse(subscription.SubscriberID) {
		return rendering{instanceRevision: instance.Revision, denied: true}, nil
	}
	descriptor, err := deps.Targets.Describe(ctx, instance)
	if err == nil {
		err = descriptor.validate(instance)
	}
	if err != nil {
		// Any provider failure makes the target unavailable for now; it must not stall fan-out.
		return rendering{instanceRevision: instance.Revision}, nil
	}
	result := rendering{instanceRevision: instance.Revision, descriptor: descriptor, available: true}
	request := processing.LayoutRequest{LayoutID: subscription.LayoutID}
	if subscription.LayoutID == nil {
		request.OutputKind = descriptor.OutputKinds[0]
	}
	resolved, err := deps.Layouts.ResolveLayout(ctx, tx, request)
	if err != nil {
		if platform.FailureCode(err) == platform.CodeInvalidInput || errors.Is(err, authz.ErrNotFound) || errors.Is(err, authz.ErrInvalidInput) {
			result.code = CodeInputInvalid
			return result, nil
		}
		return rendering{}, err
	}
	if resolved.ID != uuid.Nil {
		id := resolved.ID
		result.layoutID = &id
	}
	result.layoutVersion = resolved.Version
	if !slices.Contains(descriptor.OutputKinds, resolved.Layout.OutputKind) {
		result.code = CodeInputInvalid
		return result, nil
	}
	program, err := processing.CompileLayout(ctx, resolved.Layout, current.declarations)
	if err == nil {
		var rendered processing.RenderedLayout
		rendered, err = program.Render(ctx, current)
		if err == nil {
			result.input = &DeliveryInput{OutputKind: rendered.OutputKind, Body: string(rendered.Body)}
		}
	}
	if err != nil {
		switch platform.FailureCode(err) {
		case platform.CodeResourceLimit:
			result.code = CodeRenderLimit
			return result, nil
		case platform.CodeInvalidInput:
			result.code = CodeInputInvalid
			return result, nil
		}
		return rendering{}, err
	}
	inputs, code, err := resolveInputs(ctx, tx, deps, descriptor, target, current.units)
	if err != nil {
		return rendering{}, err
	}
	if code != "" {
		result.input, result.code = nil, code
		return result, nil
	}
	result.input.Inputs = inputs
	return result, nil
}

// resolveInputs applies the Subscription target constants, then the Unit attribute bindings, then
// the Unit parameters of the Delivery's Units, for the inputs the descriptor declares, and validates the result. Units that
// disagree on a value yield parameter-conflict; an invalid or missing value yields input-invalid.
func resolveInputs(ctx context.Context, tx platform.TransactionContext, deps Dependencies, descriptor TargetDescriptor,
	target TargetReference, units []readmodel.UnitRecord) (map[string]processing.Value, string, error) {
	inputs := map[string]processing.Value{}
	for name, value := range target.Inputs {
		if _, declared := descriptor.input(name); declared {
			inputs[name] = value
		}
	}
	if code := applyBindings(inputs, descriptor, target, units); code != "" {
		return nil, code, nil
	}
	if len(units) > 0 {
		ids := make([]uuid.UUID, 0, len(units))
		for _, unit := range units {
			ids = append(ids, unit.UnitID)
		}
		parameters, err := deps.Store.UnitParameters(ctx, tx, target.InstanceID, ids)
		if err != nil {
			return nil, "", err
		}
		for _, field := range descriptor.Inputs {
			var chosen *processing.Value
			for _, id := range ids {
				value, ok := parameters[id][field.Name]
				if !ok {
					continue
				}
				if chosen != nil && !sameValue(*chosen, value) {
					return nil, CodeParameterConflict, nil
				}
				chosen = &value
			}
			if chosen != nil {
				inputs[field.Name] = *chosen
			}
		}
	}
	for _, field := range descriptor.Inputs {
		value, ok := inputs[field.Name]
		if !ok {
			if field.Required {
				return nil, CodeInputInvalid, nil
			}
			continue
		}
		if validInput(field, value) != nil {
			return nil, CodeInputInvalid, nil
		}
	}
	return inputs, "", nil
}

// validInput checks a value against its input declaration and size bound.
func validInput(field InputField, value processing.Value) error {
	if _, err := value.Native(processing.FieldSchema{Kind: field.Kind}); err != nil {
		return err
	}
	limit := field.MaxBytes
	if limit <= 0 || limit > MaxBodyBytes {
		limit = MaxBodyBytes
	}
	if len(value.Value) > limit {
		return platform.NewFailure(platform.CodeResourceLimit, errors.New("input value too large"))
	}
	return nil
}

// expiry returns when a Delivery created now for a split stops being sendable.
func expiry(subscription Subscription, part split, now time.Time) time.Time {
	deadline := now.Add(FanOutWindow)
	if subscription.MaxAgeSeconds != nil {
		aged := part.anchor.Add(time.Duration(*subscription.MaxAgeSeconds) * time.Second)
		if aged.Before(deadline) {
			deadline = aged
		}
	}
	return deadline
}

// newRecord builds a Delivery for one split and target from its rendering.
func newRecord(subscription Subscription, current *subject, part split, target TargetReference, result rendering,
	now, expires time.Time) DeliveryRecord {
	units := make([]uuid.UUID, 0, len(part.units))
	for _, unit := range part.units {
		units = append(units, unit.UnitID)
	}
	causation := current.event.ID
	if current.event.CausationID != nil {
		causation = *current.event.CausationID
	}
	record := DeliveryRecord{
		Delivery: Delivery{ID: uuid.New(), Revision: 1, Origin: OriginAutomatic, SubscriptionID: subscription.ID,
			SubscriptionRevision: subscription.Revision, SubscriberID: subscription.SubscriberID,
			DomainEventID: current.event.ID, EventType: current.event.Type, TargetInstanceID: target.InstanceID,
			SplitKey: part.key, State: Prepared, LayoutID: result.layoutID, LayoutVersion: result.layoutVersion,
			ExpiresAt: expires, CreatedAt: now, UpdatedAt: now},
		Input: result.input, UnitIDs: units, AgencyIDs: slices.Clone(part.agencies), AgeAnchor: part.anchor,
		AccessRevision: current.authority.Revision, InstanceRevision: result.instanceRevision, CorrelationID: current.event.CorrelationID, CausationID: causation, Due: now,
	}
	if current.message != nil {
		id := current.message.Item.MessageID
		record.MessageID = &id
	}
	switch {
	case !result.available:
		// The fan-out node may simply lack the target's plugin, so the Delivery stays due now: a
		// node that can serve the target claims it at once and materializes it at start, while
		// other nodes take it only after the placement grace and defer it then.
		code := CodeDependency
		record.OutcomeCode = &code
	case result.code != "":
		code := result.code
		record.State, record.OutcomeCode, record.Input = Failed, &code, nil
		finished := now
		record.FinishedAt = &finished
	}
	return record
}

// materialize builds the Deliveries one Subscription creates for a fact now: one per split and
// target whose filter matches and whose expiry lies ahead. A Subscription whose filter or Layout
// does not compile now, or whose subscriber can no longer read their fields, becomes invalid
// and creates none.
func materialize(ctx context.Context, tx platform.TransactionContext, deps Dependencies, now time.Time,
	subscription Subscription, event eventView, memo *eventMemo) ([]DeliveryRecord, error) {
	current, err := memo.subject(ctx, tx, deps, event, subscription.SubscriberID)
	if err != nil || current == nil {
		return nil, err
	}
	check, reason, err := healthCheck(ctx, tx, deps, subscription, current.groups)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		return nil, markInvalid(ctx, tx, deps, subscription, reason, now)
	}
	if current, err = current.project(ctx, tx, deps, now, subscription); err != nil || current == nil {
		return nil, err
	}
	var records []DeliveryRecord
	for _, part := range current.splits {
		expires := expiry(subscription, part, now)
		if !expires.After(now) {
			continue
		}
		partView := current.view(part, check.declarations)
		if !matches(check.filter, partView) {
			continue
		}
		for _, target := range subscription.Targets {
			result, err := render(ctx, tx, deps, subscription, target, partView)
			if err != nil {
				return nil, err
			}
			if result.denied {
				continue
			}
			records = append(records, newRecord(subscription, current, part, target, result, now, expires))
		}
	}
	return records, nil
}

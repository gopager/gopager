package main

import (
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/assignmentstore"
	"github.com/gopager/gopager/internal/persistence/ent/commandstore"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/persistence/ent/readstore"
	"github.com/gopager/gopager/internal/persistence/ent/unitstore"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/units"
)

// core is the store set and the services shared by the API, worker and correction-runner
// compositions. It is built once per process so that every store and service exists exactly
// once; the role compositions only add what is specific to them.
type core struct {
	store *postgres.Store

	identity    *identitystore.Store
	units       *unitstore.Store
	assignments *assignmentstore.Store
	receipts    *commandstore.Store
	facts       *eventstore.Store
	intake      *ingeststore.Store
	processing  *processingstore.Store
	reads       *readstore.Store

	resolver *identity.Resolver
	// unitService manages master data.
	unitService *units.Service
	// rules matches and administers address rules against the production Network owner
	// (the intake store); the correction runner and the Processing source use it as their matcher.
	rules *assignments.Service
	// searchAttempts is the per-minute search admission limit; zero selects the default.
	searchAttempts int
	// live carries the wake hub and replay window of the Message live stream; the stream route
	// exists only when the process composed an event runtime and set it.
	live *liveConfig
	// readSide is the Message read service and search admission shared by the query, saved-view
	// and live routes, composed once so that they share one classification cache.
	readSide *readSide
}

// composeCore builds every store and the services every role needs. Constructors report
// missing dependencies as errors, so a wiring mistake fails the process at startup.
func composeCore(store *postgres.Store) (*core, error) {
	c := &core{store: store}
	var err error
	if c.identity, err = identitystore.New(store); err != nil {
		return nil, err
	}
	if c.units, err = unitstore.New(store); err != nil {
		return nil, err
	}
	if c.assignments, err = assignmentstore.New(store); err != nil {
		return nil, err
	}
	if c.receipts, err = commandstore.New(store); err != nil {
		return nil, err
	}
	if c.facts, err = eventstore.New(store); err != nil {
		return nil, err
	}
	if c.intake, err = ingeststore.New(store, c.facts, c.facts); err != nil {
		return nil, err
	}
	if c.processing, err = processingstore.New(store); err != nil {
		return nil, err
	}
	if c.reads, err = readstore.New(store); err != nil {
		return nil, err
	}
	if c.resolver, err = identity.NewResolver(c.identity, store, applicationScopeOwners{
		identity: c.identity, units: c.units, gateways: c.intake,
	}); err != nil {
		return nil, err
	}
	if c.unitService, err = units.NewService(store, c.resolver, store, c.units, c.assignments,
		c.receipts, c.facts); err != nil {
		return nil, err
	}
	if c.rules, err = c.newRuleService(c.intake); err != nil {
		return nil, err
	}
	return c, nil
}

// newRuleService builds an assignment service over the given Network owner. Production uses
// only core.rules; the Processing preview and the tests supply a different owner, whose
// matching rules must not leak into the production service.
func (c *core) newRuleService(networks assignments.NetworkOwner) (*assignments.Service, error) {
	return assignments.NewService(c.store, c.resolver, c.store, c.assignments, c.units,
		networks, c.receipts, c.facts, c.facts)
}

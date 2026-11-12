package ingeststore

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/persistence/ent/attributestore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing/declarations"
	"github.com/gopager/gopager/internal/units"
)

// UnitAttributeReader reads the Unit attributes a Workflow can read from its targets, for the
// source of a run and for a synthetic preview.
type UnitAttributeReader struct {
	store      *Store
	registries *attributes.RegistrySource
}

// NewUnitAttributeReader returns a UnitAttributeReader over the store's catalog.
func NewUnitAttributeReader(store *Store) (*UnitAttributeReader, error) {
	if store == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Unit attribute store"))
	}
	catalog, err := attributestore.New(store.primary)
	if err != nil {
		return nil, err
	}
	registries, err := attributes.NewRegistrySource(catalog)
	if err != nil {
		return nil, err
	}
	return &UnitAttributeReader{store: store, registries: registries}, nil
}

// readUnitAttributes reads the attributes of the Units the matched targets resolve to.
func (o *ProcessingSourceOwner) readUnitAttributes(ctx context.Context, tx platform.TransactionContext,
	matched assignments.MatchResult,
) (declarations.UnitAttributes, error) {
	return o.unitAttributes.ReadUnitAttributes(ctx, tx, matchedUnits(matched.Resolutions))
}

// matchedUnits returns the Units the resolutions assign.
func matchedUnits(resolutions []assignments.Resolution) []uuid.UUID {
	var ids []uuid.UUID
	for _, resolution := range resolutions {
		if resolution.UnitID != nil {
			ids = append(ids, *resolution.UnitID)
		}
	}
	return ids
}

// ReadUnitAttributes declares the Unit attributes of the catalog visible in tx and loads the
// effective attributes of the given Units in one query. It queries nothing when no Unit
// attribute is declared or no Unit is given.
func (r *UnitAttributeReader) ReadUnitAttributes(ctx context.Context, tx platform.TransactionContext,
	ids []uuid.UUID,
) (declarations.UnitAttributes, error) {
	registry, err := r.registries.Registry(ctx, tx)
	if err != nil {
		return declarations.UnitAttributes{}, err
	}
	source := declarations.UnitAttributes{Declared: declarations.UnitAttributeDeclarations(registry),
		Effective: map[uuid.UUID]units.EffectiveAttributes{}}
	if len(source.Declared) == 0 || len(ids) == 0 {
		return source, nil
	}
	executor, err := r.store.primary.Resolve(tx)
	if err != nil {
		return declarations.UnitAttributes{}, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT u.id,u.settings,o.defaults,
		(SELECT COALESCE(json_agg(sa.schema_key),'[]'::json) FROM attribute_schema_agencies sa WHERE sa.agency_id=u.agency_id)
		FROM units u LEFT JOIN organizations o ON o.id=u.organization_id WHERE u.id=ANY($1::uuid[])`, ids)
	if err != nil {
		return declarations.UnitAttributes{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var settingsJSON, assignedJSON []byte
		var defaultsJSON []byte
		if err := rows.Scan(&id, &settingsJSON, &defaultsJSON, &assignedJSON); err != nil {
			return declarations.UnitAttributes{}, err
		}
		effective, err := effectiveAttributes(settingsJSON, defaultsJSON, assignedJSON)
		if err != nil {
			return declarations.UnitAttributes{}, err
		}
		source.Effective[id] = effective
	}
	return source, rows.Err()
}

// effectiveAttributes resolves the stored settings of a Unit, the defaults of its Organization
// (nil without one) and the schema keys assigned to its Agency.
func effectiveAttributes(settingsJSON, defaultsJSON, assignedJSON []byte) (units.EffectiveAttributes, error) {
	var settings units.Settings
	if err := json.Unmarshal(settingsJSON, &settings); err != nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, err)
	}
	var defaults *units.Defaults
	if defaultsJSON != nil {
		defaults = &units.Defaults{}
		if err := json.Unmarshal(defaultsJSON, defaults); err != nil {
			return nil, platform.NewFailure(platform.CodeInvalidInput, err)
		}
	}
	var keys []string
	if err := json.Unmarshal(assignedJSON, &keys); err != nil {
		return nil, err
	}
	assigned := make(map[string]bool, len(keys))
	for _, key := range keys {
		assigned[key] = true
	}
	return units.ResolveSettings(settings, defaults, func(key string) bool { return assigned[key] }).Attributes, nil
}

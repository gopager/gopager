package units

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

func equalUUIDPointer(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func (s *Service) authorizeOrganizationMembers(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, action string, id uuid.UUID, now time.Time) (Organization, []Unit, []UnitTemplate, error) {
	organization, members, templates, err := s.repository.ReadOrganizationContext(ctx, tx, id)
	if err != nil {
		return Organization{}, nil, nil, err
	}
	if len(members) == 0 {
		if err := authz.Authorize(authority, action, nil, authz.Target{}, now); err != nil {
			return Organization{}, nil, nil, err
		}
		return organization, members, templates, nil
	}
	for _, member := range members {
		if err := s.authorizeUnitUses(ctx, tx, authority, action, member, now); err != nil {
			return Organization{}, nil, nil, authz.ErrForbidden
		}
	}
	return organization, members, templates, nil
}

func (s *Service) authorizeTemplateReference(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, id *uuid.UUID, now time.Time) error {
	if id == nil {
		return nil
	}
	return s.authorizeOrganizationReference(ctx, tx, authority, *id, now)
}

func (s *Service) recordTemplate(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, value UnitTemplate, change, action string, command *uuid.UUID, now time.Time) error {
	actor, err := uuid.Parse(authority.PrincipalID)
	if err != nil {
		return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
	}
	if err := s.repository.RecordChange(ctx, tx, actor, value.ID, action, now); err != nil {
		return err
	}
	_, err = s.facts.AppendMasterDataFact(ctx, tx, events.MasterDataFact{ID: uuid.New(),
		Aggregate: events.EntityRef{Type: "unitTemplate", ID: value.ID}, Revision: value.Revision,
		OccurredAt: now, CausationID: command, ChangeKind: change})
	return err
}

// ReadOrganization returns configuration only when every current Unit use is manageable.
func (s *Service) ReadOrganization(ctx context.Context, proof identity.Proof, id uuid.UUID) (Organization, error) {
	if id == uuid.Nil {
		return Organization{}, authz.ErrInvalidInput
	}
	var result Organization
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		var err error
		result, err = s.repository.ReadOrganization(tx.Context(), tx, authority, "organization.read", id, now)
		return err
	})
	if err != nil {
		return Organization{}, err
	}
	return result, nil
}

// ListOrganizations returns only rows whose complete current Unit context is in scope.
func (s *Service) ListOrganizations(ctx context.Context, proof identity.Proof, page PageInput) (Page[Organization], error) {
	var result Page[Organization]
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		var err error
		result, err = s.repository.ListOrganizations(tx.Context(), tx, authority, page, now)
		return err
	})
	if err != nil {
		return Page[Organization]{}, err
	}
	return result, nil
}

// CreateOrganization creates a complete defaults resource with an actor-bound retry receipt.
func (s *Service) CreateOrganization(ctx context.Context, proof identity.Proof, command uuid.UUID, input OrganizationInput, check func(identity.Authentication) error) (commands.Result, error) {
	input, err := input.Canonical()
	if err != nil || command == uuid.Nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	fingerprint, err := commands.Fingerprint(input)
	if err != nil {
		return commands.Result{}, err
	}
	var result commands.Result
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if err := authz.Authorize(authority, "organization.create", nil, authz.Target{}, now); err != nil {
			return err
		}
		actor, err := uuid.Parse(authority.PrincipalID)
		if err != nil {
			return err
		}
		receipt, found, err := s.commands.ReadCommandReceipt(tx.Context(), tx, actor, command)
		if err != nil {
			return err
		}
		if found {
			result, err = receipt.Check("organization.create", fingerprint)
			return err
		}
		value := Organization{ID: uuid.New(), Revision: 1, Name: input.Name, Defaults: input.Defaults}
		if err := s.repository.SaveOrganization(tx.Context(), tx, authority, value, 0, now); err != nil {
			return err
		}
		if err := s.record(tx.Context(), tx, authority, value.ID, value.Revision, "organization",
			"created", "organization.create", &command, now); err != nil {
			return err
		}
		result = commands.Result{ID: value.ID, Revision: value.Revision}
		return s.commands.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: actor, CommandID: command,
			Operation: "organization.create", Fingerprint: fingerprint, Result: result, RecordedAt: now})
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

// ChangeOrganization replaces defaults only after checking every current Unit use.
func (s *Service) ChangeOrganization(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64, input OrganizationInput, check func(identity.Authentication) error) (commands.Result, error) {
	input, err := input.Canonical()
	if err != nil || id == uuid.Nil || expected <= 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		current, _, _, err := s.authorizeOrganizationMembers(tx.Context(), tx, authority, "organization.update", id, now)
		if err != nil {
			return err
		}
		next, err := commands.NextRevision(current.Revision, expected)
		if err != nil {
			return err
		}
		if current.Name == input.Name && reflect.DeepEqual(current.Defaults, input.Defaults) {
			result = commands.Result{ID: id, Revision: current.Revision}
			return nil
		}
		change := "updated"
		if !reflect.DeepEqual(current.Defaults, input.Defaults) {
			change = "defaults-changed"
		}
		current.Name, current.Defaults, current.Revision = input.Name, input.Defaults, next
		if err := s.repository.SaveOrganization(tx.Context(), tx, authority, current, expected, now); err != nil {
			return err
		}
		if err := s.record(tx.Context(), tx, authority, id, next, "organization", change, "organization.update", nil, now); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: next}
		return nil
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

// DeleteOrganization detaches and materializes every current reference atomically.
func (s *Service) DeleteOrganization(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64, check func(identity.Authentication) error) (commands.Result, error) {
	if id == uuid.Nil || expected <= 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err := s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		organization, members, templates, err := s.authorizeOrganizationMembers(tx.Context(), tx, authority, "organization.delete", id, now)
		if err != nil {
			return err
		}
		next, err := commands.NextRevision(organization.Revision, expected)
		if err != nil {
			return err
		}
		for i := range members {
			if err := s.authorizeUnitUses(tx.Context(), tx, authority, "unit.update", members[i], now); err != nil {
				return authz.ErrForbidden
			}
			revision, err := commands.NextRevision(members[i].Revision, members[i].Revision)
			if err != nil {
				return err
			}
			members[i].Settings = MaterializeSettings(members[i].Settings, organization.Defaults)
			members[i].OrganizationID, members[i].Revision = nil, revision
		}
		for i := range templates {
			if err := authz.Authorize(authority, "unit-template.update", nil, authz.Target{}, now); err != nil {
				return authz.ErrForbidden
			}
			revision, err := commands.NextRevision(templates[i].Revision, templates[i].Revision)
			if err != nil {
				return err
			}
			templates[i].Settings = MaterializeSettings(templates[i].Settings, organization.Defaults)
			templates[i].OrganizationID, templates[i].Revision = nil, revision
		}
		for _, member := range members {
			if err := s.repository.SaveUnit(tx.Context(), tx, authority, member, member.Revision-1, now); err != nil {
				return err
			}
			if err := s.record(tx.Context(), tx, authority, member.ID, member.Revision,
				"unit", "updated", "unit.update", nil, now); err != nil {
				return err
			}
		}
		for _, template := range templates {
			if err := s.repository.SaveUnitTemplate(tx.Context(), tx, template, template.Revision-1); err != nil {
				return err
			}
			if err := s.recordTemplate(tx.Context(), tx, authority, template, "updated", "unit-template.update", nil, now); err != nil {
				return err
			}
		}
		organization.Revision = next
		if err := s.repository.DeleteOrganization(tx.Context(), tx, authority, organization, expected, now); err != nil {
			return err
		}
		if err := s.record(tx.Context(), tx, authority, id, next, "organization", "deleted", "organization.delete", nil, now); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: next}
		return nil
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

func templateSettingsFromEffective(settings EffectiveSettings) Settings {
	return Settings{
		Color:            settingFromEffective(settings.Color),
		Icon:             settingFromEffective(settings.Icon),
		FunctionMeanings: settingFromEffective(settings.FunctionMeanings),
	}
}

func settingFromEffective[T any](value EffectiveValue[T]) FieldSetting[T] {
	if value.Value == nil {
		return FieldSetting[T]{Mode: "unset"}
	}
	copy := *value.Value
	return FieldSetting[T]{Mode: "value", Value: &copy}
}

// CreateUnitTemplate stores a standalone template with explicit all-scope authorization.
func (s *Service) CreateUnitTemplate(ctx context.Context, proof identity.Proof, command uuid.UUID, input UnitTemplateInput, check func(identity.Authentication) error) (commands.Result, error) {
	input, err := input.Canonical()
	if err != nil || command == uuid.Nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	fingerprint, err := commands.Fingerprint(input)
	if err != nil {
		return commands.Result{}, err
	}
	var result commands.Result
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if err := authz.Authorize(authority, "unit-template.create", nil, authz.Target{}, now); err != nil {
			return err
		}
		if err := s.authorizeTemplateReference(tx.Context(), tx, authority, input.OrganizationID, now); err != nil {
			return err
		}
		actor, err := uuid.Parse(authority.PrincipalID)
		if err != nil {
			return err
		}
		receipt, found, err := s.commands.ReadCommandReceipt(tx.Context(), tx, actor, command)
		if err != nil {
			return err
		}
		if found {
			result, err = receipt.Check("unit-template.create", fingerprint)
			return err
		}
		value := UnitTemplate{ID: uuid.New(), Revision: 1, Name: input.Name,
			OrganizationID: input.OrganizationID, Settings: input.Settings}
		if err := s.repository.SaveUnitTemplate(tx.Context(), tx, value, 0); err != nil {
			return err
		}
		if err := s.recordTemplate(tx.Context(), tx, authority, value, "created", "unit-template.create", &command, now); err != nil {
			return err
		}
		result = commands.Result{ID: value.ID, Revision: value.Revision}
		return s.commands.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: actor, CommandID: command,
			Operation: "unit-template.create", Fingerprint: fingerprint, Result: result, RecordedAt: now})
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

// ReadUnitTemplate requires the explicit all-scope template read action.
func (s *Service) ReadUnitTemplate(ctx context.Context, proof identity.Proof, id uuid.UUID) (UnitTemplate, error) {
	if id == uuid.Nil {
		return UnitTemplate{}, authz.ErrInvalidInput
	}
	var result UnitTemplate
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if err := authz.Authorize(authority, "unit-template.read", nil, authz.Target{}, now); err != nil {
			return authz.ErrNotFound
		}
		var err error
		result, err = s.repository.ReadUnitTemplate(tx.Context(), tx, id)
		return err
	})
	if err != nil {
		return UnitTemplate{}, err
	}
	return result, nil
}

// ListUnitTemplates returns a bounded page after the explicit all-scope check.
func (s *Service) ListUnitTemplates(ctx context.Context, proof identity.Proof, page PageInput) (Page[UnitTemplate], error) {
	var result Page[UnitTemplate]
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if err := authz.Authorize(authority, "unit-template.read", nil, authz.Target{}, now); err != nil {
			return err
		}
		var err error
		result, err = s.repository.ListUnitTemplates(tx.Context(), tx, page)
		return err
	})
	if err != nil {
		return Page[UnitTemplate]{}, err
	}
	return result, nil
}

// ChangeUnitTemplate replaces a standalone template under revision control.
func (s *Service) ChangeUnitTemplate(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64, input UnitTemplateInput, check func(identity.Authentication) error) (commands.Result, error) {
	input, err := input.Canonical()
	if err != nil || id == uuid.Nil || expected <= 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if err := authz.Authorize(authority, "unit-template.update", nil, authz.Target{}, now); err != nil {
			return err
		}
		current, err := s.repository.ReadUnitTemplate(tx.Context(), tx, id)
		if err != nil {
			return err
		}
		next, err := commands.NextRevision(current.Revision, expected)
		if err != nil {
			return err
		}
		if err := s.authorizeTemplateReference(tx.Context(), tx, authority, input.OrganizationID, now); err != nil {
			return err
		}
		if current.Name == input.Name && equalUUIDPointer(current.OrganizationID, input.OrganizationID) &&
			reflect.DeepEqual(current.Settings, input.Settings) {
			result = commands.Result{ID: id, Revision: current.Revision}
			return nil
		}
		current.Name, current.OrganizationID, current.Settings = input.Name, input.OrganizationID, input.Settings
		current.Revision = next
		if err := s.repository.SaveUnitTemplate(tx.Context(), tx, current, expected); err != nil {
			return err
		}
		if err := s.recordTemplate(tx.Context(), tx, authority, current, "updated", "unit-template.update", nil, now); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: next}
		return nil
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

// DeleteUnitTemplate marks a revisioned tombstone; copied Units have no template edge.
func (s *Service) DeleteUnitTemplate(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64, check func(identity.Authentication) error) (commands.Result, error) {
	if id == uuid.Nil || expected <= 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err := s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if err := authz.Authorize(authority, "unit-template.delete", nil, authz.Target{}, now); err != nil {
			return err
		}
		current, err := s.repository.ReadUnitTemplate(tx.Context(), tx, id)
		if err != nil {
			return err
		}
		next, err := commands.NextRevision(current.Revision, expected)
		if err != nil {
			return err
		}
		current.Deleted, current.Revision = true, next
		if err := s.repository.SaveUnitTemplate(tx.Context(), tx, current, expected); err != nil {
			return err
		}
		if err := s.recordTemplate(tx.Context(), tx, authority, current, "deleted", "unit-template.delete", nil, now); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: next}
		return nil
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

// CreateUnitFromTemplate copies the current template snapshot and returns no live template link.
func (s *Service) CreateUnitFromTemplate(ctx context.Context, proof identity.Proof, command uuid.UUID, templateID uuid.UUID, input UnitInput, check func(identity.Authentication) error) (commands.Result, error) {
	if templateID == uuid.Nil || input.NewOrganization != nil || input.OrganizationID != nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	input, err := input.Canonical()
	if err != nil || command == uuid.Nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	fingerprint, err := commands.Fingerprint(struct {
		TemplateID uuid.UUID `json:"templateId"`
		Input      UnitInput `json:"input"`
	}{templateID, input})
	if err != nil {
		return commands.Result{}, err
	}
	var result commands.Result
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if err := authz.Authorize(authority, "unit-template.read", nil, authz.Target{}, now); err != nil {
			return err
		}
		actor, err := uuid.Parse(authority.PrincipalID)
		if err != nil {
			return err
		}
		receipt, found, err := s.commands.ReadCommandReceipt(tx.Context(), tx, actor, command)
		if err != nil {
			return err
		}
		template, err := s.repository.ReadUnitTemplate(tx.Context(), tx, templateID)
		if err != nil {
			return err
		}
		if err := s.authorizeTemplateReference(tx.Context(), tx, authority, template.OrganizationID, now); err != nil {
			return err
		}
		id := uuid.New()
		if found {
			id = receipt.Result.ID
		}
		if err := authz.Authorize(authority, "unit.create", nil,
			authz.Target{AgencyID: input.AgencyID, UnitID: id, UnitCollections: input.CollectionIDs}, now); err != nil {
			return err
		}
		if err := s.repository.LockAgency(tx.Context(), tx, input.AgencyID); err != nil {
			return err
		}
		if err := s.checkMembership(tx.Context(), tx, authority, nil, input.CollectionIDs, now); err != nil {
			return err
		}
		if found {
			result, err = receipt.Check("unit.from-template", fingerprint)
			if err != nil {
				return err
			}
			return nil
		}
		value := Unit{ID: id, Revision: 1, Name: input.Name, AgencyID: input.AgencyID, OrganizationID: template.OrganizationID,
			CollectionIDs: input.CollectionIDs, Settings: templateSettingsFromEffective(template.EffectiveSettings)}
		if err := s.repository.SaveUnit(tx.Context(), tx, authority, value, 0, now); err != nil {
			return err
		}
		if err := s.record(tx.Context(), tx, authority, id, 1, "unit", "created", "unit.create", &command, now); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: 1}
		return s.commands.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: actor, CommandID: command,
			Operation: "unit.from-template", Fingerprint: fingerprint, Result: result, RecordedAt: now})
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

func canonicalBulkInput(input BulkUnitInput) (BulkUnitInput, error) {
	if (input.Operation != "update" && input.Operation != "delete") || len(input.Items) == 0 || len(input.Items) > 100 {
		return BulkUnitInput{}, authz.ErrInvalidInput
	}
	input.Items = slices.Clone(input.Items)
	slices.SortFunc(input.Items, func(a, b BulkUnitItem) int { return strings.Compare(a.ID.String(), b.ID.String()) })
	for i := range input.Items {
		item := &input.Items[i]
		if item.ID == uuid.Nil || item.ExpectedRevision <= 0 || (i > 0 && item.ID == input.Items[i-1].ID) {
			return BulkUnitInput{}, authz.ErrInvalidInput
		}
		if input.Operation == "update" {
			if item.Settings == nil {
				return BulkUnitInput{}, authz.ErrInvalidInput
			}
			settings, err := item.Settings.Canonical()
			if err != nil {
				return BulkUnitInput{}, err
			}
			item.Settings = &settings
		} else if item.Settings != nil {
			return BulkUnitInput{}, authz.ErrInvalidInput
		}
	}
	return input, nil
}

func (s *Service) prepareBulk(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, input BulkUnitInput, now time.Time) ([]Unit, error) {
	ids := make([]uuid.UUID, len(input.Items))
	for i, item := range input.Items {
		ids[i] = item.ID
	}
	action := "unit.update"
	if input.Operation == "delete" {
		action = "unit.delete"
	}
	values, err := s.repository.LockUnits(ctx, tx, authority, action, ids, now)
	if err != nil {
		return nil, err
	}
	if len(values) != len(input.Items) {
		return nil, authz.ErrNotFound
	}
	for i := range values {
		item := input.Items[i]
		if values[i].ID != item.ID {
			return nil, authz.ErrNotFound
		}
		if _, err := commands.NextRevision(values[i].Revision, item.ExpectedRevision); err != nil {
			return nil, err
		}
		if err := s.authorizeUnitUses(ctx, tx, authority, action, values[i], now); err != nil {
			return nil, authz.ErrNotFound
		}
	}
	return values, nil
}

// PreviewBulkUnits validates a complete bounded operation without durable
// writes. A non-nil check is applied to the resolved caller before any read, so
// transports can enforce request-bound protections such as Origin and CSRF for
// Cookie Sessions.
func (s *Service) PreviewBulkUnits(ctx context.Context, proof identity.Proof, input BulkUnitInput,
	check func(identity.Authentication) error,
) (BulkPreview, error) {
	input, err := canonicalBulkInput(input)
	if err != nil {
		return BulkPreview{}, err
	}
	result := BulkPreview{Valid: true, Items: make([]BulkItemPreview, len(input.Items))}
	err = s.within(ctx, proof, false, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		action := "unit.update"
		if input.Operation == "delete" {
			action = "unit.delete"
		}
		for i, item := range input.Items {
			preview := BulkItemPreview{ID: item.ID, ExpectedRevision: item.ExpectedRevision, Valid: false}
			value, err := s.repository.ReadUnit(tx.Context(), tx, authority, action, item.ID, now)
			if errors.Is(err, authz.ErrNotFound) || errors.Is(err, authz.ErrForbidden) {
				preview.Code = "NOT_FOUND"
			} else if err != nil {
				return err
			} else if err := s.authorizeUnitUses(tx.Context(), tx, authority, action, value, now); errors.Is(err, authz.ErrForbidden) {
				preview.Code = "NOT_FOUND"
			} else if err != nil {
				return err
			} else if _, err := commands.NextRevision(value.Revision, item.ExpectedRevision); errors.Is(err, authz.ErrRevisionConflict) {
				preview.Code = "REVISION_CONFLICT"
			} else if err != nil {
				preview.Code = "INVALID_INPUT"
			} else {
				preview.Valid = true
			}
			if !preview.Valid {
				result.Valid = false
			}
			result.Items[i] = preview
		}
		return nil
	})
	if err != nil {
		return BulkPreview{}, err
	}
	return result, nil
}

// BulkUnits applies a validated bounded update/delete as one receipt and transaction.
func (s *Service) BulkUnits(ctx context.Context, proof identity.Proof, command uuid.UUID, input BulkUnitInput, check func(identity.Authentication) error) ([]BulkUnitResult, error) {
	input, err := canonicalBulkInput(input)
	if err != nil || command == uuid.Nil {
		return nil, authz.ErrInvalidInput
	}
	fingerprint, err := commands.Fingerprint(input)
	if err != nil {
		return nil, err
	}
	var result []BulkUnitResult
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		actor, err := uuid.Parse(authority.PrincipalID)
		if err != nil {
			return err
		}
		receipt, found, err := s.commands.ReadCommandReceipt(tx.Context(), tx, actor, command)
		if err != nil {
			return err
		}
		if found {
			replay, err := receipt.Check("unit.bulk."+input.Operation, fingerprint)
			if err != nil {
				return err
			}
			for _, item := range input.Items {
				current, err := s.repository.ReadUnitContext(tx.Context(), tx, item.ID)
				if err != nil {
					return authz.ErrNotFound
				}
				target := Unit{ID: current.ID, CollectionIDs: current.CollectionIDs, Deleted: current.Deleted}
				action := "unit.update"
				if input.Operation == "delete" {
					action = "unit.delete"
				}
				if err := s.authorizeUnitUses(tx.Context(), tx, authority, action, target, now); err != nil {
					return authz.ErrNotFound
				}
			}
			if len(replay.Items) != len(input.Items) {
				return platform.NewFailure(platform.CodeAuthorityUnavailable, authz.ErrInvalidInput)
			}
			result = make([]BulkUnitResult, len(replay.Items))
			for i, item := range replay.Items {
				result[i] = BulkUnitResult{ID: item.ID, Revision: item.Revision, Status: item.Status}
			}
			return nil
		}
		values, err := s.prepareBulk(tx.Context(), tx, authority, input, now)
		if err != nil {
			return err
		}
		result = make([]BulkUnitResult, len(values))
		for i, value := range values {
			item := input.Items[i]
			next, err := commands.NextRevision(value.Revision, item.ExpectedRevision)
			if err != nil {
				return err
			}
			change, action := "bulk-updated", "unit.update"
			if input.Operation == "update" {
				if reflect.DeepEqual(value.Settings, *item.Settings) {
					result[i] = BulkUnitResult{ID: value.ID, Revision: value.Revision, Status: "updated"}
					continue
				}
				value.Settings = *item.Settings
			} else {
				value.Deleted, change, action = true, "deleted", "unit.delete"
			}
			value.Revision = next
			if err := s.repository.SaveUnit(tx.Context(), tx, authority, value, item.ExpectedRevision, now); err != nil {
				return err
			}
			if input.Operation == "delete" {
				if err := s.rules.InvalidateUnit(tx.Context(), tx, value.ID); err != nil {
					return err
				}
			}
			if err := s.record(tx.Context(), tx, authority, value.ID, value.Revision, "unit", change, action, &command, now); err != nil {
				return err
			}
			status := "updated"
			if input.Operation == "delete" {
				status = "pending"
			}
			result[i] = BulkUnitResult{ID: value.ID, Revision: value.Revision, Status: status}
		}
		receiptItems := make([]commands.ResultItem, len(result))
		if len(result) > 0 {
			for i, item := range result {
				receiptItems[i] = commands.ResultItem{ID: item.ID, Revision: item.Revision, Status: item.Status}
			}
		}
		receiptResult := commands.Result{ID: result[0].ID, Revision: result[0].Revision, Items: receiptItems}
		return s.commands.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: actor, CommandID: command,
			Operation: "unit.bulk." + input.Operation, Fingerprint: fingerprint, Result: receiptResult, RecordedAt: now})
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

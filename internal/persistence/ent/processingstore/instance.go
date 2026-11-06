package processingstore

import (
	"context"
	"errors"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/processinginstance"
	"github.com/gopager/gopager/internal/persistence/ent/processinginstanceversion"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

var _ processing.InstanceRepository = (*Store)(nil)

type currentInstanceProjection struct {
	InstanceID    uuid.UUID                        `sql:"instance_id"`
	Revision      int64                            `sql:"revision"`
	Configuration processing.InstanceConfiguration `sql:"configuration"`
}

func currentInstanceJoin(selector *sql.Selector) {
	parent := sql.Table(processinginstance.Table)
	selector.Join(parent).On(
		selector.C(processinginstanceversion.FieldInstanceID),
		parent.C(processinginstance.FieldID),
	)
	selector.Where(sql.ColumnsEQ(
		selector.C(processinginstanceversion.FieldRevision),
		parent.C(processinginstance.FieldRevision),
	))
}

func (s *Store) currentInstanceReadClient(ctx context.Context, tx platform.TransactionContext,
	authority authz.Authority, now time.Time) (*ent.Client, context.Context, error) {
	if now.IsZero() {
		return nil, nil, authz.ErrInvalidInput
	}
	if err := authz.Authorize(authority, "plugin.configure", nil, authz.Target{}, now); err != nil {
		return nil, nil, err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"ProcessingInstance": entql.Not(entql.FieldEQ("id", uuid.Nil)),
		"ProcessingInstanceVersion": entql.Not(
			entql.FieldEQ("instance_id", uuid.Nil),
		),
	}, nil)
	if err != nil {
		return nil, nil, err
	}
	return client, scope, nil
}

// ReadCurrentInstance returns only the version selected by the current instance pointer.
func (s *Store) ReadCurrentInstance(ctx context.Context, tx platform.TransactionContext,
	authority authz.Authority, id uuid.UUID, now time.Time) (processing.InstanceConfiguration, error) {
	if id == uuid.Nil {
		return processing.InstanceConfiguration{}, authz.ErrInvalidInput
	}
	client, scope, err := s.currentInstanceReadClient(ctx, tx, authority, now)
	if err != nil {
		return processing.InstanceConfiguration{}, err
	}
	var rows []currentInstanceProjection
	err = client.ProcessingInstanceVersion.Query().
		Where(processinginstanceversion.InstanceID(id)).
		Modify(currentInstanceJoin).
		Select(processinginstanceversion.FieldInstanceID, processinginstanceversion.FieldRevision,
			processinginstanceversion.FieldConfiguration).
		Scan(scope, &rows)
	if err != nil {
		return processing.InstanceConfiguration{}, err
	}
	if len(rows) == 0 {
		return processing.InstanceConfiguration{}, authz.ErrNotFound
	}
	if len(rows) != 1 {
		return processing.InstanceConfiguration{}, platform.NewFailure(platform.CodeAuthorityUnavailable,
			errors.New("current instance query returned multiple rows"))
	}
	value := rows[0].Configuration
	if value.ID != id || value.Revision != rows[0].Revision || rows[0].InstanceID != id {
		return processing.InstanceConfiguration{}, platform.NewFailure(platform.CodeInternalError,
			errors.New("current instance snapshot identity mismatch"))
	}
	return value, value.Validate()
}

// ListInstances returns only current instance snapshots with native pointer filtering before paging.
func (s *Store) ListInstances(ctx context.Context, tx platform.TransactionContext,
	authority authz.Authority, input processing.PageInput, now time.Time,
) (processing.Page[processing.InstanceConfiguration], error) {
	input, err := page(input)
	if err != nil {
		return processing.Page[processing.InstanceConfiguration]{}, err
	}
	client, scope, err := s.currentInstanceReadClient(ctx, tx, authority, now)
	if err != nil {
		return processing.Page[processing.InstanceConfiguration]{}, err
	}
	total, err := client.ProcessingInstance.Query().Count(scope)
	if err != nil {
		return processing.Page[processing.InstanceConfiguration]{}, err
	}
	var rows []currentInstanceProjection
	err = client.ProcessingInstanceVersion.Query().
		Order(processinginstanceversion.ByInstanceID()).
		Offset(input.Offset).Limit(input.Limit).
		Modify(currentInstanceJoin).
		Select(processinginstanceversion.FieldInstanceID, processinginstanceversion.FieldRevision,
			processinginstanceversion.FieldConfiguration).
		Scan(scope, &rows)
	if err != nil {
		return processing.Page[processing.InstanceConfiguration]{}, err
	}
	result := processing.Page[processing.InstanceConfiguration]{
		Total: total, Items: make([]processing.InstanceConfiguration, 0, len(rows)),
	}
	for _, row := range rows {
		value := row.Configuration
		if value.ID != row.InstanceID || value.Revision != row.Revision {
			return processing.Page[processing.InstanceConfiguration]{}, platform.NewFailure(
				platform.CodeInternalError, errors.New("current instance snapshot identity mismatch"))
		}
		if err := value.Validate(); err != nil {
			return processing.Page[processing.InstanceConfiguration]{}, err
		}
		result.Items = append(result.Items, value)
	}
	return result, nil
}

// ReadInstance returns only the exact current revision to a trusted invocation/configuration owner.
// Historical snapshots remain private evidence, not permission to start disabled or replaced modules.
func (s *Store) ReadInstance(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, revision int64) (processing.InstanceConfiguration, error) {
	if id == uuid.Nil || revision <= 0 {
		return processing.InstanceConfiguration{}, authz.ErrInvalidInput
	}
	if _, err := s.LockRevision(ctx, tx); err != nil {
		return processing.InstanceConfiguration{}, err
	}
	key := entql.And(entql.FieldEQ("id", id), entql.FieldEQ("revision", revision))
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"ProcessingInstance": key,
		"ProcessingInstanceVersion": entql.And(entql.FieldEQ("instance_id", id),
			entql.FieldEQ("revision", revision)),
	}, nil)
	if err != nil {
		return processing.InstanceConfiguration{}, err
	}
	if _, err := client.ProcessingInstance.Query().Only(scope); err != nil {
		if ent.IsNotFound(err) {
			return processing.InstanceConfiguration{}, authz.ErrRevisionConflict
		}
		return processing.InstanceConfiguration{}, err
	}
	row, err := client.ProcessingInstanceVersion.Query().Only(scope)
	if err != nil {
		return processing.InstanceConfiguration{}, err
	}
	value := row.Configuration
	if value.ID != id || value.Revision != revision {
		return processing.InstanceConfiguration{}, platform.NewFailure(platform.CodeInternalError,
			errors.New("instance snapshot identity mismatch"))
	}
	return value, value.Validate()
}

// SaveInstance atomically appends an immutable configuration and advances the current pointer.
// Module inspection/current execution-Principal checks happen in the coordinating owner.
func (s *Store) SaveInstance(ctx context.Context, tx platform.TransactionContext, authority authz.Authority,
	value processing.InstanceConfiguration, expected int64) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if _, err := s.LockRevisionForChange(ctx, tx); err != nil {
		return err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return err
	}
	if err := authz.Authorize(authority, "plugin.configure", nil, authz.Target{}, now); err != nil {
		return err
	}
	actor, err := uuid.Parse(authority.PrincipalID)
	if err != nil || actor != value.CreatedBy || value.CreatedAt.After(now) {
		return authz.ErrInvalidInput
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"ProcessingInstance": entql.FieldEQ("id", value.ID),
	}, map[string]bool{"ProcessingInstance": true, "ProcessingInstanceVersion": true})
	if err != nil {
		return err
	}
	current, err := client.ProcessingInstance.Query().ForUpdate().Only(scope)
	if ent.IsNotFound(err) {
		if expected != 0 || value.Revision != 1 {
			return authz.ErrRevisionConflict
		}
		if err := client.ProcessingInstance.Create().SetID(value.ID).SetRevision(1).Exec(scope); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		revision, err := commands.NextRevision(current.Revision, expected)
		if err != nil {
			return err
		}
		if value.Revision != revision {
			return authz.ErrRevisionConflict
		}
		changed, err := client.ProcessingInstance.Update().
			Where(processinginstance.ID(value.ID), processinginstance.Revision(expected)).
			SetRevision(revision).Save(scope)
		if err != nil {
			return err
		}
		if changed != 1 {
			return authz.ErrRevisionConflict
		}
	}
	if err := client.ProcessingInstanceVersion.Create().SetID(uuid.New()).SetInstanceID(value.ID).
		SetRevision(value.Revision).SetConfiguration(value).Exec(scope); err != nil {
		return err
	}
	return s.RecordChange(ctx, tx, actor, value.ID, "plugin.configure", now)
}

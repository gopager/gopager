package presentation

import (
	"context"
	"slices"
	"time"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// metadataActions are the actions of which any one lets a caller see the editor metadata: the
// ones that edit or choose Layouts.
var metadataActions = []string{ManageAction, "subscription.manage-own", "subscription.manage-global"}

// EditorMetadata describes what a Layout editor may offer.
type EditorMetadata struct {
	// BlockKinds are the block kinds with their allowed and required properties.
	BlockKinds []processing.LayoutBlockSpec `json:"blockKinds"`
	// OutputKinds are the output kinds a Layout can render to.
	OutputKinds []string `json:"outputKinds"`
	// Bounds are the structural limits of a definition and its rendering.
	Bounds processing.LayoutBounds `json:"bounds"`
	// Fields are the Delivery fields the caller may read, sorted by path.
	Fields []EditorField `json:"fields"`
}

// EditorField is one field a Layout may reference.
type EditorField struct {
	Ref   processing.FieldRef    `json:"ref"`
	Type  processing.FieldSchema `json:"type"`
	Group string                 `json:"group,omitempty"`
	Label string                 `json:"label"`
	Icon  string                 `json:"icon,omitempty"`
	// Deprecated fields still render in saved Layouts but cannot be newly referenced.
	Deprecated bool `json:"deprecated"`
}

// EditorMetadata returns the block kinds, output kinds, bounds and the Delivery fields of the
// registry that the caller may read. It needs presentation.manage, subscription.manage-own or
// subscription.manage-global (authz.ErrForbidden otherwise). A field of a group the caller may
// not read is left out, so the editor never reveals declarations the caller could not render.
func (s *Service) EditorMetadata(ctx context.Context, proof identity.Proof) (EditorMetadata, error) {
	result := EditorMetadata{BlockKinds: processing.LayoutBlockSpecs(), OutputKinds: processing.LayoutOutputKinds(),
		Bounds: processing.LayoutLimits(), Fields: []EditorField{}}
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, who caller, _ time.Time) error {
		if _, err := authz.VisibleAgencies(who.authority, metadataActions, true); err != nil {
			return err
		}
		readable, err := authz.ReadableFieldGroups(who.authority)
		if err != nil {
			return err
		}
		registry, err := s.fields.Registry(tx.Context(), tx)
		if err != nil {
			return err
		}
		for _, field := range registry.ListAll(fields.ScopeDelivery) {
			if field.Group != "" && !slices.Contains(readable, field.Group) {
				continue
			}
			result.Fields = append(result.Fields, EditorField{Ref: field.Ref, Type: field.Type, Group: field.Group,
				Label: field.Label, Icon: field.Icon, Deprecated: field.Deprecated})
		}
		return nil
	})
	return result, err
}

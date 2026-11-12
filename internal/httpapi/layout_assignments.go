package httpapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/processing/presentation"
	"github.com/gopager/gopager/internal/units"
)

// LayoutAssignmentService is the layout assignment side the routes call.
// *presentation.AssignmentService implements it.
type LayoutAssignmentService interface {
	List(context.Context, identity.Proof, string, uuid.UUID, units.PageInput) (units.Page[presentation.Assignment], error)
	Put(context.Context, identity.Proof, string, uuid.UUID, string, uuid.UUID, int64, func(identity.Authentication) error) (presentation.Assignment, error)
	Delete(context.Context, identity.Proof, string, uuid.UUID, string, int64, func(identity.Authentication) error) (commands.Result, error)
}

// layoutAssignmentPut is the body of PUT /layout-assignments/{scopeKind}/{scopeId}/{outputKind}.
// ExpectedRevision is 0 (or absent) to create and the current revision to replace.
type layoutAssignmentPut struct {
	SchemaVersion    int       `json:"schemaVersion"`
	ExpectedRevision int64     `json:"expectedRevision,string"`
	LayoutID         uuid.UUID `json:"layoutId"`
}

// layoutAssignmentDelete is the body of DELETE /layout-assignments/{scopeKind}/{scopeId}/{outputKind}.
type layoutAssignmentDelete struct {
	SchemaVersion    int   `json:"schemaVersion"`
	ExpectedRevision int64 `json:"expectedRevision,string"`
}

// RegisterLayoutAssignmentRoutes adds the layout assignment routes. The service checks the rights.
func RegisterLayoutAssignmentRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator Authenticator, assignments LayoutAssignmentService) {
	guard := routeGuard{transport: transport, authenticator: authenticator}
	mux.HandleFunc("GET /api/v1/layout-assignments", guard.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, filters, err := managementPageWith(r, "scopeKind", "scopeId")
		if err != nil {
			return nil, err
		}
		var scopeID uuid.UUID
		if value, ok := filters["scopeId"]; ok {
			if scopeID, err = uuid.Parse(value); err != nil || scopeID == uuid.Nil {
				return nil, authz.InvalidField("scopeId")
			}
		}
		return assignments.List(r.Context(), proof, filters["scopeKind"], scopeID, page)
	}, http.StatusOK))
	mux.HandleFunc("PUT /api/v1/layout-assignments/{scopeKind}/{scopeId}/{outputKind}", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input layoutAssignmentPut
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		scopeID, err := managementPathID(r, "scopeId")
		if err != nil || checkSchemaVersion(input.SchemaVersion) != nil {
			return nil, authz.ErrInvalidInput
		}
		return assignments.Put(r.Context(), proof, r.PathValue("scopeKind"), scopeID, r.PathValue("outputKind"),
			input.LayoutID, input.ExpectedRevision, check)
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/layout-assignments/{scopeKind}/{scopeId}/{outputKind}", guard.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input layoutAssignmentDelete
		if err := decodeManagementJSON(r, &input); err != nil {
			return nil, err
		}
		scopeID, err := managementPathID(r, "scopeId")
		if err != nil || checkSchemaVersion(input.SchemaVersion) != nil {
			return nil, authz.ErrInvalidInput
		}
		return assignments.Delete(r.Context(), proof, r.PathValue("scopeKind"), scopeID, r.PathValue("outputKind"),
			input.ExpectedRevision, check)
	}, http.StatusOK))
}

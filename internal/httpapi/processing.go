package httpapi

import (
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	processingmanagement "github.com/gopager/gopager/internal/processing/management"
)

// ProcessingRoutes carries the Processing management services behind the HTTP routes. A nil
// service leaves its routes unregistered, so a deployment without it answers 404 rather than
// failing at request time.
type ProcessingRoutes struct {
	Workflows *processingmanagement.Service
	Reads     *processingmanagement.ProcessingReads
	Retries   *processingmanagement.RetryService
	Preview   *processingmanagement.PreviewService
	Instances *processingmanagement.InstanceService
	// Reprocess serves the historical Reprocess request and progress routes of a Workflow.
	Reprocess *processingmanagement.ReprocessService
	// Personal and UnitParameters serve the plugin configuration routes of Delivery instances.
	Personal       *processingmanagement.PersonalService
	UnitParameters *processingmanagement.UnitParameterService
}

type processingWorkflowAPI struct {
	routeGuard
	service        *processingmanagement.Service
	reads          *processingmanagement.ProcessingReads
	retries        *processingmanagement.RetryService
	reprocess      *processingmanagement.ReprocessService
	preview        *processingmanagement.PreviewService
	instances      *processingmanagement.InstanceService
	personal       *processingmanagement.PersonalService
	unitParameters *processingmanagement.UnitParameterService
}

// decodeProcessingJSON uses the kernel's closed codec, including duplicate-key and size checks.
func decodeProcessingJSON(r *http.Request, input any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return authz.ErrInvalidInput
	}
	raw, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, processing.MaxInputBytes))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			return platform.NewFailure(platform.CodeResourceLimit, err)
		}
		return platform.NewFailure(platform.CodeInvalidInput, err)
	}
	return processing.Decode(raw, input)
}

// RegisterProcessingRoutes adds the Workflow, Instance, candidate read, retry and preview routes
// for every service present in routes.
func RegisterProcessingRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator Authenticator, routes ProcessingRoutes) {
	api := processingWorkflowAPI{routeGuard: routeGuard{transport: transport, authenticator: authenticator}, service: routes.Workflows,
		reads: routes.Reads, retries: routes.Retries, reprocess: routes.Reprocess, preview: routes.Preview, instances: routes.Instances,
		personal: routes.Personal, unitParameters: routes.UnitParameters}
	if api.service != nil {
		api.registerWorkflows(mux)
		api.registerVersions(mux)
	}
	if api.reads != nil {
		api.registerReads(mux)
	}
	if api.retries != nil {
		api.registerRetry(mux)
	}
	if api.reprocess != nil {
		api.registerReprocess(mux)
	}
	if api.preview != nil {
		api.registerPreview(mux)
	}
	if api.instances != nil {
		api.registerInstances(mux)
	}
	if api.instances != nil && api.personal != nil {
		api.registerPersonal(mux)
	}
	if api.instances != nil && api.unitParameters != nil {
		api.registerUnitParameters(mux)
	}
}

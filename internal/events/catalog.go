package events

import "slices"

// Replay modes of an envelope's replayMode. Automatic reactions such as Delivery fan-out act
// only on facts recorded in normal operation.
const (
	// ReplayNormal marks a fact recorded by ordinary operation.
	ReplayNormal = "normal"
	// ReplayReprocess marks a fact recorded by an explicit historical Reprocess; it never
	// triggers an automatic external reaction.
	ReplayReprocess = "reprocess"
)

// Projection names the authorized view a consumer must read before it may act on a fact.
type Projection string

const (
	// ProjectionNone means the fact refers to no Message or Gateway a Subscription could read.
	ProjectionNone Projection = ""
	// ProjectionMessage means the fact names a Message in data.messageId.
	ProjectionMessage Projection = "message"
	// ProjectionGateway means the fact names a Gateway in data.gatewayId.
	ProjectionGateway Projection = "gateway"
)

// CatalogEntry describes one recorded event type.
type CatalogEntry struct {
	// Type is the envelope type.
	Type string
	// Producer is the producer domain that records the type.
	Producer string
	// SchemaVersion is the only envelope schema version recorded for the type.
	SchemaVersion int
	// Deliverable reports whether a Subscription may select the type as a Delivery trigger.
	Deliverable bool
	// Projection is the authorized view a Delivery of this type is materialized from.
	Projection Projection
}

// catalog lists every event type the product records, across all producing domains, so that a
// later reaction capability can select non-deliverable types without a new vocabulary.
var catalog = []CatalogEntry{
	{Type: string(MessageCreated), Producer: "grouping", SchemaVersion: 1, Deliverable: true, Projection: ProjectionMessage},
	{Type: string(MessageTransmissionJoined), Producer: "grouping", SchemaVersion: 1, Deliverable: true, Projection: ProjectionMessage},
	{Type: string(MessageMerged), Producer: "grouping", SchemaVersion: 1, Deliverable: true, Projection: ProjectionMessage},
	{Type: string(AssignmentChanged), Producer: "assignment", SchemaVersion: 1, Deliverable: true, Projection: ProjectionMessage},
	{Type: "suppression.changed", Producer: "assignment", SchemaVersion: 1, Deliverable: true, Projection: ProjectionMessage},
	{Type: "processing.completed", Producer: "processing", SchemaVersion: 1, Deliverable: true, Projection: ProjectionMessage},
	{Type: string(GatewayStateChanged), Producer: "intake", SchemaVersion: 1, Deliverable: true, Projection: ProjectionGateway},
	{Type: "correction.failed", Producer: "assignment", SchemaVersion: 1},
	{Type: string(ConfigurationChanged), Producer: "", SchemaVersion: 1},
	{Type: string(PrincipalAccessChanged), Producer: "identity", SchemaVersion: 1},
	{Type: string(PrincipalDeleted), Producer: "identity", SchemaVersion: 1},
	{Type: string(SubmissionRejected), Producer: "intake", SchemaVersion: 1},
	{Type: string(ObservationTimeImplausible), Producer: "intake", SchemaVersion: 1},
	{Type: string(NetworkAmbiguous), Producer: "intake", SchemaVersion: 1},
}

// Catalog returns an independent copy of the event-type catalog in a stable order. A
// configuration.changed entry has no single producer: every configuration owner records it.
func Catalog() []CatalogEntry { return slices.Clone(catalog) }

// LookupType returns the catalog entry of an event type and whether it is known.
func LookupType(eventType string) (CatalogEntry, bool) {
	for _, entry := range catalog {
		if entry.Type == eventType {
			return entry, true
		}
	}
	return CatalogEntry{}, false
}

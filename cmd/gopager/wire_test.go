package main

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/processing"
	processingmanagement "github.com/gopager/gopager/internal/processing/management"
)

// The request bodies below are written out here instead of imported from internal/httpapi so
// the composed-binary tests pin the wire format independently of the adapter's own types.

type workflowPreviewMutation struct {
	SchemaVersion        int                    `json:"schemaVersion"`
	VersionID            uuid.UUID              `json:"versionId"`
	Definition           *processing.Definition `json:"definition"`
	SyntheticJSON        json.RawMessage        `json:"syntheticJson"`
	StoredTransmissionID uuid.UUID              `json:"storedTransmissionId"`
}

type instanceMutation struct {
	SchemaVersion    int       `json:"schemaVersion"`
	CommandID        uuid.UUID `json:"commandId"`
	ExpectedRevision int64     `json:"expectedRevision,string"`
	processingmanagement.InstanceInput
}

type workflowVersionMutation struct {
	SchemaVersion    int                   `json:"schemaVersion"`
	CommandID        uuid.UUID             `json:"commandId"`
	ExpectedRevision int64                 `json:"expectedRevision,string"`
	Definition       processing.Definition `json:"definition"`
}

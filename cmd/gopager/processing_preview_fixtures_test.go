package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/synthetic"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/testutil/pluginfixture"
)

// installedPreviewArtifacts provisions the command plugin twice: once whose manifest declares
// supportsPreview and once whose manifest does not.
func installedPreviewArtifacts(t *testing.T) (string, processing.ProvisionedArtifact, processing.ProvisionedArtifact) {
	t.Helper()
	root := t.TempDir()
	raw, err := os.ReadFile("../../internal/processing/testdata/instance-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest processing.InstalledManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.ResultSchema = map[string]processing.FieldSchema{"text": {Kind: processing.StringValue}}
	binary := pluginfixture.Binary(t, "command")
	digest := sha256.Sum256(binary)
	if err := os.WriteFile(filepath.Join(root, "plugin"), binary, 0o755); err != nil {
		t.Fatal(err)
	}
	artifacts := make([]processing.ProvisionedArtifact, 2)
	for index, supports := range []bool{true, false} {
		manifest.Artifact = processing.ArtifactReference{Path: "plugin", Digest: hex.EncodeToString(digest[:])}
		manifest.SupportsPreview = supports
		encoded, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		name := "supports-preview.json"
		if !supports {
			name = "no-preview.json"
		}
		if err := os.WriteFile(filepath.Join(root, name), encoded, 0600); err != nil {
			t.Fatal(err)
		}
		artifacts[index] = processing.ProvisionedArtifact{ManifestPath: name,
			Pin: processing.ArtifactPin{PluginID: manifest.PluginID, Version: manifest.Version,
				CapabilityID: manifest.CapabilityID, Digest: manifest.Artifact.Digest}}
	}
	return root, artifacts[0], artifacts[1]
}

func sourceProtocolPreviewDefinition(t *testing.T) processing.Definition {
	t.Helper()
	field := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	arguments := make([]processing.Predicate, 0, 4)
	for _, protocol := range []protocols.Protocol{
		protocols.POCSAG, protocols.FLEX, protocols.ZVEI, protocols.EAS,
	} {
		expected, err := processing.Literal(processing.StringValue, string(protocol))
		if err != nil {
			t.Fatal(err)
		}
		arguments = append(arguments, processing.Predicate{
			Op: "eq", Field: &field, Value: &expected,
		})
	}
	return processing.Definition{SchemaVersion: 1,
		Selector: processing.Predicate{Op: "or", Args: arguments},
		Outputs:  map[string]map[string]processing.Assignment{}}
}

func unitTargetPreviewDefinition(t *testing.T, unitID uuid.UUID) processing.Definition {
	t.Helper()
	field := processing.FieldRef{Namespace: "target", Path: []string{"unit", "id"}}
	expected, err := processing.Literal(processing.StringValue, unitID.String())
	if err != nil {
		t.Fatal(err)
	}
	return processing.Definition{SchemaVersion: 1,
		Selector: processing.Predicate{Op: "eq", Field: &field, Value: &expected},
		Outputs:  map[string]map[string]processing.Assignment{"unit-ref": {"unit": {Field: &field}}}}
}

func binarySourcePreviewDefinition(t *testing.T) processing.Definition {
	t.Helper()
	field := processing.FieldRef{Namespace: "source", Path: []string{"payload", "binary"}}
	expected, err := processing.Literal(processing.BytesValue, []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	return processing.Definition{SchemaVersion: 1,
		Selector: processing.Predicate{Op: "eq", Field: &field, Value: &expected},
		Outputs:  map[string]map[string]processing.Assignment{}}
}

func syntheticPreviewInputs(networks map[uuid.UUID]synthetic.Network) []synthetic.Input {
	networkByProtocol := make(map[protocols.Protocol]uuid.UUID, len(networks))
	for id, network := range networks {
		networkByProtocol[network.Protocol] = id
	}
	pocsagTarget := protocols.Target{Address: protocols.NumericAddress(123)}
	function := uint8(1)
	pocsagTarget.Function = &function
	zveiTarget := protocols.Target{Address: protocols.TextAddress("12345")}
	easTarget := protocols.Target{Address: protocols.TextAddress("123456")}
	return []synthetic.Input{
		{SchemaVersion: 1, Protocol: protocols.POCSAG, Form: "text", NetworkID: networkByProtocol[protocols.POCSAG],
			Payload: json.RawMessage(`{"value":"sample text"}`), Targets: []protocols.Target{pocsagTarget}},
		{SchemaVersion: 1, Protocol: protocols.POCSAG, Form: "numeric", NetworkID: networkByProtocol[protocols.POCSAG],
			Payload: json.RawMessage(`{"value":"123"}`), Targets: []protocols.Target{pocsagTarget}},
		{SchemaVersion: 1, Protocol: protocols.POCSAG, Form: "tone-only", NetworkID: networkByProtocol[protocols.POCSAG],
			Targets: []protocols.Target{}},
		{SchemaVersion: 1, Protocol: protocols.FLEX, Form: "text", NetworkID: networkByProtocol[protocols.FLEX],
			Payload: json.RawMessage(`{"value":"sample text"}`), Targets: []protocols.Target{}},
		{SchemaVersion: 1, Protocol: protocols.FLEX, Form: "numeric", NetworkID: networkByProtocol[protocols.FLEX],
			Payload: json.RawMessage(`{"value":"123"}`), Targets: []protocols.Target{}},
		{SchemaVersion: 1, Protocol: protocols.FLEX, Form: "tone-only", NetworkID: networkByProtocol[protocols.FLEX],
			Targets: []protocols.Target{}},
		{SchemaVersion: 1, Protocol: protocols.FLEX, Form: "binary", Subtype: "secure",
			NetworkID: networkByProtocol[protocols.FLEX], Payload: json.RawMessage(`{"encoding":"base64","data":"AQID"}`),
			Targets: []protocols.Target{}},
		{SchemaVersion: 1, Protocol: protocols.FLEX, Form: "flex-short-signal",
			NetworkID: networkByProtocol[protocols.FLEX], Payload: json.RawMessage(`{"sourceCode":3}`),
			Targets: []protocols.Target{}},
		{SchemaVersion: 1, Protocol: protocols.ZVEI, Form: "zvei-signal", NetworkID: networkByProtocol[protocols.ZVEI],
			Payload: json.RawMessage(`{"double_tone":null}`), Targets: []protocols.Target{zveiTarget}},
		{SchemaVersion: 1, Protocol: protocols.EAS, Form: "eas-header", NetworkID: networkByProtocol[protocols.EAS],
			Payload: json.RawMessage(`{"originator":"EAS","eventCode":"TOR","purgeTime":"0015","issueTime":"0010000"}`),
			Targets: []protocols.Target{easTarget}},
	}
}

func processingPreviewSideEffects(t *testing.T, store *postgres.Store) [3]int64 {
	t.Helper()
	var counts [3]int64
	err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT
			(SELECT count(*) FROM processing_candidates),
			(SELECT count(*) FROM processing_results),
			(SELECT count(*) FROM domain_events)`).
			Scan(&counts[0], &counts[1], &counts[2])
	})
	if err != nil {
		t.Fatal(err)
	}
	return counts
}

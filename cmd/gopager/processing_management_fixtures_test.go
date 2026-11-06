package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/account"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/grant"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/unitstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/synthetic"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/testutil/pluginfixture"
)

type previewFixtureNetworks struct {
	values   map[uuid.UUID]synthetic.Network
	identity *identitystore.Store
	units    *unitstore.Store
}

func (owner previewFixtureNetworks) ReadSyntheticNetwork(ctx context.Context, _ platform.TransactionContext,
	id uuid.UUID) (synthetic.Network, error) {
	if err := ctx.Err(); err != nil {
		return synthetic.Network{}, err
	}
	network, exists := owner.values[id]
	if !exists {
		return synthetic.Network{}, authz.ErrNotFound
	}
	return network, nil
}

func (owner previewFixtureNetworks) ReadNetwork(ctx context.Context, tx platform.TransactionContext,
	id uuid.UUID) (protocols.Protocol, uuid.UUID, error) {
	network, err := owner.ReadSyntheticNetwork(ctx, tx, id)
	if err != nil {
		return "", uuid.Nil, err
	}
	return network.Protocol, network.AgencyID, nil
}

// fixtureDigest is the SHA-256 of the command plugin fixture built for this test process.
func fixtureDigest(t *testing.T) string {
	t.Helper()
	sum := sha256.Sum256(pluginfixture.Binary(t, "command"))
	return hex.EncodeToString(sum[:])
}

// pluginFixtureRoot provisions the built command plugin and the testdata manifests, pinned to it.
func pluginFixtureRoot(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "plugin"), pluginfixture.Binary(t, "command"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"instance-manifest.json", "instance-redaction-manifest.json"} {
		raw, err := os.ReadFile("../../internal/processing/testdata/" + path)
		if err != nil {
			t.Fatal(err)
		}
		var manifest processing.InstalledManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			t.Fatal(err)
		}
		manifest.Artifact.Digest = fixtureDigest(t)
		encoded, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, path), encoded, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return directory
}

func revokeManagementAction(t *testing.T, store *postgres.Store, principalID uuid.UUID, action string) {
	t.Helper()
	err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{
			Current: driver.Current,
			Predicates: map[string]entql.P{
				"Grant":           entql.FieldEQ("subject_id", principalID),
				"IdentityControl": entql.FieldEQ("id", 1),
			},
		})
		grants, err := client.Grant.Query().Where(grant.SubjectID(principalID)).All(scope)
		if err != nil {
			return err
		}
		for _, record := range grants {
			actions := make([]string, 0, len(record.Actions))
			for _, current := range record.Actions {
				if current != action {
					actions = append(actions, current)
				}
			}
			if len(actions) == len(record.Actions) {
				continue
			}
			if err := client.Grant.UpdateOneID(record.ID).SetActions(actions).AddRevision(1).Exec(scope); err != nil {
				return err
			}
		}
		return client.IdentityControl.UpdateOneID(1).AddAccessRevision(1).Exec(scope)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func managementOperatorID(t *testing.T, store *postgres.Store) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{
			Current: driver.Current,
			Predicates: map[string]entql.P{
				"Account": entql.FieldEQ("username_key", "operator"),
			},
		})
		row, err := client.Account.Query().Where(account.UsernameKey("operator")).Only(scope)
		if err != nil {
			return err
		}
		id = row.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func seedCurrentInstance(t *testing.T, store *postgres.Store, value processing.InstanceConfiguration) {
	t.Helper()
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{
			Current: driver.Current,
			Create: map[string]bool{
				"ProcessingInstance":        true,
				"ProcessingInstanceVersion": true,
			},
		})
		if err := client.ProcessingInstance.Create().SetID(value.ID).SetRevision(value.Revision).Exec(scope); err != nil {
			return err
		}
		return client.ProcessingInstanceVersion.Create().SetID(uuid.New()).SetInstanceID(value.ID).
			SetRevision(value.Revision).SetConfiguration(value).Exec(scope)
	})
	if err != nil {
		t.Fatal(err)
	}
}

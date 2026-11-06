package management

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/processing"
)

func TestInstanceSummaryOmitsLocalPathsDigestsAndConfiguration(t *testing.T) {
	t.Parallel()
	digest := sha256.Sum256([]byte("pinned artifact"))
	configuration := processing.InstanceConfiguration{
		ID: uuid.New(), Revision: 3, Enabled: true, ExecutionPrincipalID: uuid.New(),
		Main: processing.ProvisionedArtifact{
			ManifestPath: "local/secret/path.json",
			Pin: processing.ArtifactPin{PluginID: "plug", Version: "v1", CapabilityID: "main",
				Digest: hex.EncodeToString(digest[:])},
		},
		Configuration: processing.Value{Kind: processing.StringValue, Value: json.RawMessage(`"private-config"`)},
		CreatedAt:     time.Now().UTC().Truncate(time.Microsecond), CreatedBy: uuid.New(),
	}
	summary, err := instanceSummary(configuration)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	for _, privateValue := range []string{
		"local/secret/path.json", hex.EncodeToString(digest[:]), "private-config", configuration.CreatedBy.String(),
	} {
		if strings.Contains(string(encoded), privateValue) {
			t.Fatalf("private instance value %q appeared in summary: %s", privateValue, encoded)
		}
	}
}

func TestInstancePageBounds(t *testing.T) {
	t.Parallel()
	for _, input := range []processing.PageInput{
		{Offset: -1, Limit: 1},
		{Limit: 0},
		{Limit: processing.MaxCollection + 1},
		{Offset: int(^uint(0) >> 1), Limit: 1},
	} {
		if err := validateInstancePage(input); err == nil {
			t.Fatalf("invalid instance page accepted: %+v", input)
		}
	}
	if err := validateInstancePage(processing.PageInput{Offset: 3, Limit: processing.MaxCollection}); err != nil {
		t.Fatalf("valid bounded page rejected: %v", err)
	}
}

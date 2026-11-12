package management

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// SecretInput is the write for one Secret property: exactly one of a new plaintext Value (sealed
// into the encrypted store) or a Reference to an operator-provided file or environment variable.
// A JSON null in the Secrets map removes the Secret instead.
type SecretInput struct {
	Value     *string `json:"value"`
	Reference *string `json:"reference"`
}

// SecretWriter stores sealed Secret rows in the caller's transaction, after the instance row that
// owns them exists.
type SecretWriter interface {
	InsertSecrets(ctx context.Context, tx platform.TransactionContext, rows []processing.StoredSecret) error
}

// secretSettings holds the Secret collaborators of the services; the cipher is nil when the
// operator configured no master key.
type secretSettings struct {
	cipher *processing.SecretCipher
	writer SecretWriter
}

// secretPlan is the outcome of a Secret write: the reference string for every configured Secret
// path and the sealed rows to store with the change.
type secretPlan struct {
	references map[string]string
	rows       []processing.StoredSecret
}

// secretScope says whose Secrets a write handles.
type secretScope struct {
	// scope is the configuration scope of the handled properties: "admin" or "user".
	scope string
	// instanceID and owner bind sealed rows; owner is nil for administrative values.
	instanceID uuid.UUID
	owner      *uuid.UUID
	// allowReference admits file and environment references; personal values may only be stored.
	allowReference bool
}

// maxSecretValueBytes bounds a plaintext Secret like a referenced Secret file.
const maxSecretValueBytes = processing.MaxSecretFileBytes

func secretUnavailable(cause string) error {
	return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New(cause))
}

// validate checks the input's shape for the scope.
func (i SecretInput) validate(allowReference bool) error {
	if (i.Value == nil) == (i.Reference == nil) {
		return authz.ErrInvalidInput
	}
	if i.Value != nil {
		if len(*i.Value) == 0 || len(*i.Value) > maxSecretValueBytes || !utf8.ValidString(*i.Value) {
			return authz.ErrInvalidInput
		}
		return nil
	}
	if !allowReference {
		return authz.ErrInvalidInput
	}
	reference, err := processing.ParseSecretReference(*i.Reference)
	if err != nil || reference.Source == processing.SecretStored {
		return authz.ErrInvalidInput
	}
	return nil
}

// secretFingerprints reduces the Secrets of a command to values safe for its fingerprint: a keyed
// digest of a plaintext, the reference string, or a removal marker. A plaintext without a
// configured key is unavailable, like every other use of the Secret store.
func secretFingerprints(cipher *processing.SecretCipher, inputs map[string]*SecretInput) (map[string]string, error) {
	result := make(map[string]string, len(inputs))
	for path, input := range inputs {
		switch {
		case input == nil:
			result[path] = "remove"
		case input.Value != nil && input.Reference == nil:
			if cipher == nil {
				return nil, secretUnavailable("secret key is not configured")
			}
			result[path] = "value:" + cipher.Fingerprint([]byte(path+"\x00"+*input.Value))
		case input.Reference != nil && input.Value == nil:
			result[path] = "reference:" + *input.Reference
		default:
			return nil, authz.ErrInvalidInput
		}
	}
	return result, nil
}

// planSecrets applies the Secret writes of one command to the references of the previous state.
// A path absent from inputs keeps its previous reference; null removes it (only for optional
// properties); a value is sealed into a new row; a reference is taken over after checking its
// form. Paths the manifest does not declare as Secrets of the scope are invalid input.
func planSecrets(settings secretSettings, manifest processing.InstalledManifest, target secretScope,
	previous map[string]string, inputs map[string]*SecretInput) (secretPlan, error) {
	declared := map[string]processing.SecretProperty{}
	for _, property := range manifest.SecretProperties() {
		if property.Scope == target.scope {
			declared[property.Path] = property
		}
	}
	plan := secretPlan{references: map[string]string{}}
	for path, reference := range previous {
		if _, known := declared[path]; known {
			plan.references[path] = reference
		}
	}
	for path, input := range inputs {
		property, known := declared[path]
		if !known {
			return secretPlan{}, authz.ErrInvalidInput
		}
		if input == nil {
			if !property.Optional {
				return secretPlan{}, authz.ErrInvalidInput
			}
			delete(plan.references, path)
			continue
		}
		if err := input.validate(target.allowReference); err != nil {
			return secretPlan{}, err
		}
		if input.Reference != nil {
			plan.references[path] = *input.Reference
			continue
		}
		if settings.cipher == nil {
			return secretPlan{}, secretUnavailable("secret key is not configured")
		}
		row, err := settings.cipher.Seal(uuid.New(), target.instanceID, target.owner, []byte(*input.Value))
		if err != nil {
			return secretPlan{}, err
		}
		plan.rows = append(plan.rows, row)
		plan.references[path] = "secret:" + row.ID.String()
	}
	return plan, nil
}

// previousReferences extracts the reference strings of an earlier state through the manifest that
// state was saved with. A state that cannot be read keeps nothing: the caller must then supply
// every Secret again.
func previousReferences(manifest processing.InstalledManifest, value processing.Value) map[string]string {
	references, err := manifest.SecretReferenceStrings(value)
	if err != nil {
		return nil
	}
	return references
}

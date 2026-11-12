package management

import (
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/processing"
)

// requireReadableGroups requires the editor to read every field group of the source fields a
// definition reads, so that a preview of a stored Transmission never shows a field the editor
// could not read on the Transmission itself. The failure is authz.ErrForbidden.
func requireReadableGroups(authority authz.Authority, references []processing.SourceReference) error {
	for _, reference := range references {
		for _, group := range reference.Groups {
			if err := authz.AuthorizeFields(authority, []string{group}); err != nil {
				return err
			}
		}
	}
	return nil
}

package management

import (
	"slices"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/processing"
)

// requireReadableGroups requires the editor to read every field group of the source fields a
// definition reads, so that a preview of a stored Transmission never shows a field the editor
// could not read on the Transmission itself. The failure is authz.ErrForbidden.
func requireReadableGroups(authority authz.Authority, references []processing.SourceReference) error {
	var groups []string
	for _, reference := range references {
		groups = append(groups, reference.Groups...)
	}
	if len(groups) == 0 {
		return nil
	}
	slices.Sort(groups)
	return authz.AuthorizeFields(authority, slices.Compact(groups))
}

package authz

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// AuthorizeInAnyAgency requires the action in at least one of the Agencies. It is the rule for
// referencing a definition that belongs to several Agencies: holding the right in one of them
// suffices. The result is ErrForbidden when the list is empty or no Agency permits the action,
// and any other error an Authorize call returns.
func AuthorizeInAnyAgency(authority Authority, action string, agencies []uuid.UUID, now time.Time) error {
	for _, agency := range agencies {
		err := Authorize(authority, action, nil, Target{AgencyID: agency}, now)
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrForbidden) {
			return err
		}
	}
	return ErrForbidden
}

package httpapi

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/units"
)

// managementPage reads the optional offset and limit query parameters; any other parameter,
// a repeated one or a non-integer value is authz.ErrInvalidInput.
func managementPage(r *http.Request) (units.PageInput, error) {
	page, _, err := managementPageWith(r)
	return page, err
}

// managementPageWith reads offset and limit like managementPage and additionally returns the
// listed filter parameters as given. Each parameter may appear once; any other parameter is
// authz.ErrInvalidInput.
func managementPageWith(r *http.Request, filters ...string) (units.PageInput, map[string]string, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return units.PageInput{}, nil, authz.ErrInvalidInput
	}
	page := units.PageInput{Limit: 50}
	found := map[string]string{}
	for name, entries := range values {
		if len(entries) != 1 {
			return units.PageInput{}, nil, authz.ErrInvalidInput
		}
		if slices.Contains(filters, name) {
			found[name] = entries[0]
			continue
		}
		value, err := strconv.Atoi(entries[0])
		if err != nil {
			return units.PageInput{}, nil, authz.ErrInvalidInput
		}
		switch name {
		case "offset":
			page.Offset = value
		case "limit":
			if value < 1 {
				return units.PageInput{}, nil, authz.ErrInvalidInput
			}
			page.Limit = value
		default:
			return units.PageInput{}, nil, authz.ErrInvalidInput
		}
	}
	page, err = page.Validate()
	return page, found, err
}

// managementID rejects absent/non-UUID resource paths before a repository is called.
func managementID(r *http.Request) (uuid.UUID, error) {
	return managementPathID(r, "id")
}

// managementPathID rejects an absent, non-UUID or nil path parameter before a repository is called.
func managementPathID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, authz.ErrInvalidInput
	}
	return id, nil
}

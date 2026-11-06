package httpapi

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/units"
)

// managementPage parses only the supported bounded page parameters, never ignored filters.
func managementPage(r *http.Request) (units.PageInput, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return units.PageInput{}, authz.ErrInvalidInput
	}
	page := units.PageInput{Limit: 50}
	for name, entries := range values {
		if len(entries) != 1 {
			return units.PageInput{}, authz.ErrInvalidInput
		}
		value, err := strconv.Atoi(entries[0])
		if err != nil {
			return units.PageInput{}, authz.ErrInvalidInput
		}
		switch name {
		case "offset":
			page.Offset = value
		case "limit":
			if value < 1 {
				return units.PageInput{}, authz.ErrInvalidInput
			}
			page.Limit = value
		default:
			return units.PageInput{}, authz.ErrInvalidInput
		}
	}
	return page.Validate()
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

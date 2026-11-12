package readstore

import "github.com/gopager/gopager/internal/readmodel"

// Statement renders the SQL text and parameters that Feed (or Detail when detail is set) executes
// for the query, so that measurements can explain exactly the statement of production.
func Statement(query readmodel.FeedQuery, detail bool) (string, []any) {
	p := &params{}
	builder := &feedBuilder{params: p, elig: newEligibility(p, query.Access), query: query, detail: detail}
	return builder.statement(), p.values
}

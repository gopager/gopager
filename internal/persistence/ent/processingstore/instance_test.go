package processingstore

import (
	"strings"
	"testing"

	"entgo.io/ent/dialect/sql"
	"github.com/gopager/gopager/internal/persistence/ent/processinginstance"
	"github.com/gopager/gopager/internal/persistence/ent/processinginstanceversion"
)

func TestCurrentInstanceJoinMatchesTheCurrentRevision(t *testing.T) {
	t.Parallel()
	selector := sql.Dialect("postgres").Select().From(sql.Table(processinginstanceversion.Table))
	currentInstanceJoin(selector)
	query, args := selector.Query()
	if len(args) != 0 || !strings.Contains(query, processinginstance.Table) ||
		!strings.Contains(query, processinginstanceversion.Table) ||
		!strings.Contains(query, processinginstanceversion.FieldRevision) ||
		!strings.Contains(query, processinginstance.FieldRevision) {
		t.Fatalf("current instance query lacks its pointer join: %q, args=%v", query, args)
	}
}

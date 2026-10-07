package routes

import (
	"net/http"
	"testing"

	"mycorrhizal/internal/fireandforget"

	"github.com/stretchr/testify/require"
)

// TestDBFaultSweep_AuthMiddlewareFailure covers the statements the per-route
// sweep starts after (faultAuthPrefix): when the session lookup itself fails,
// the request is refused (never served as authenticated), nothing is written,
// and nothing SQL leaks through the middleware's bare {"error": ...} envelope.
func TestDBFaultSweep_AuthMiddlewareFailure(t *testing.T) {
	for i, stmt := range faultAuthPrefix {
		e := newFaultEnv(t)
		res := seedResources(t, e.db, e.owner.ID)
		fireandforget.Wait()
		before := snapshotDB(t, e.db)

		e.inj.FailNth(i + 1)
		w := e.do(t, faultReq{method: http.MethodDelete, path: "/api/v1/contacts/" + res.contact})
		fired, ok := e.inj.Fired()
		e.inj.Disarm()
		fireandforget.Wait()

		require.True(t, ok, "%s: injection did not fire", stmt)
		require.Equal(t, stmt, fired.String())
		require.GreaterOrEqual(t, w.Code, 401, "%s: %s", stmt, w.Body.String())
		require.Empty(t, faultLeak(w.Body.String(), underscoreTables(t, e.db)), "%s: %s", stmt, w.Body.String())
		require.Empty(t, diffSnapshots(before, snapshotDB(t, e.db)), "%s: a refused request wrote", stmt)
	}
}

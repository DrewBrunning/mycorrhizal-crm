package services

import (
	"fmt"

	"github.com/rs/zerolog"
)

// importRowFailure builds the per-row error string returned to the client in
// ImportResult.Errors. The underlying database error goes to the server log
// only: the DB-fault sweep (issue #1476) found that interpolating err into the
// string leaked the raw driver/SQL error text into the HTTP 200 body.
func importRowFailure(log *zerolog.Logger, row int, what string, err error) string {
	log.Error().Err(err).Int("row", row).Msgf("Import row failed to %s", what)
	return fmt.Sprintf("Row %d: Failed to %s", row, what)
}

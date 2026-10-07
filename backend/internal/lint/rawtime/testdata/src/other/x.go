package other

import "time"

// Outside controllers/services/middleware: not in scope.
func f() { _ = time.Now() }

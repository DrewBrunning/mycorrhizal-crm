package middleware

import "time"

func budgeted() {
	_ = time.Now()
	_ = time.Now()
	_ = time.Now() // want `raw time\.Now\(\) in middleware`
}

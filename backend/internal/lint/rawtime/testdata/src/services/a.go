package services

import (
	"fmt"
	stdtime "time"
)

func positives() {
	_ = stdtime.Now()                 // want `raw time\.Now\(\) in services`
	_ = stdtime.Since(stdtime.Time{}) // want `raw time\.Since\(\) in services`
	_ = stdtime.Until(stdtime.Time{}) // want `raw time\.Until\(\) in services`
	register(stdtime.Now)             // want `raw time\.Now\(\) in services`
}

func register(f func() stdtime.Time) { fmt.Println(f != nil) }

func negatives() {
	_ = stdtime.Date(2026, 1, 1, 0, 0, 0, 0, stdtime.UTC)
	_ = stdtime.Duration(3) * stdtime.Second
	var t stdtime.Time
	_ = t.Add(stdtime.Hour)
	_ = t.Sub(t)
}

func inlineAllowed() {
	start := stdtime.Now() // rawtime:allow elapsed-duration measurement
	// rawtime:allow comment-only line covers the next line
	_ = stdtime.Since(start)
}

func markerWithoutReason() {
	_ = stdtime.Now() /* rawtime:allow */ // want `needs a reason after the marker` `raw time\.Now\(\) in services`
}

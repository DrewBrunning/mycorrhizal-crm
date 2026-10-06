package flakeledger

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ParseRerunReport reads a gotestsum --rerun-fails-report file. Each line is
// "<pkg>.<Test>: N runs, M failures". M < N means a later attempt passed
// (passed on retry); M == N means every attempt failed.
func ParseRerunReport(r io.Reader) (Observations, error) {
	obs := Observations{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		i := strings.LastIndex(line, ": ")
		if i < 0 {
			continue
		}
		var runs, fails int
		if n, _ := fmt.Sscanf(line[i+2:], "%d runs, %d failures", &runs, &fails); n != 2 || fails == 0 {
			continue
		}
		o := PassedOnRetry
		if fails >= runs {
			o = Failed
		}
		obs.record(line[:i], o)
	}
	return obs, sc.Err()
}

type junitCase struct {
	Classname string    `xml:"classname,attr"`
	Name      string    `xml:"name,attr"`
	Failure   *struct{} `xml:"failure"`
	Error     *struct{} `xml:"error"`
	Skipped   *struct{} `xml:"skipped"`
}

// ParseJUnit streams a JUnit XML file. A test seen both failing and passing
// (gotestsum records every attempt) is PassedOnRetry; only failing is Failed.
// Skipped cases are ignored.
func ParseJUnit(r io.Reader) (Observations, error) {
	passed := map[string]bool{}
	failed := map[string]bool{}
	d := xml.NewDecoder(r)
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("junit: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "testcase" {
			continue
		}
		var tc junitCase
		if err := d.DecodeElement(&tc, &se); err != nil {
			return nil, fmt.Errorf("junit: %w", err)
		}
		if tc.Skipped != nil {
			continue
		}
		id := tc.Name
		if tc.Classname != "" {
			id = tc.Classname + "." + tc.Name
		}
		if tc.Failure != nil || tc.Error != nil {
			failed[id] = true
		} else {
			passed[id] = true
		}
	}
	obs := Observations{}
	for id := range passed {
		obs[id] = Passed
	}
	for id := range failed {
		if passed[id] {
			obs[id] = PassedOnRetry
		} else {
			obs[id] = Failed
		}
	}
	return obs, nil
}

// MergeAttempts combines an earlier attempt's JUnit (first) with the final
// attempt's (final), the Android shape: attempt 1's XML is preserved as
// flaky-attempt-1 and the surviving results are the retry's. A test that
// failed attempt 1 and passes in the final run is PassedOnRetry; one that is
// failing, or absent, in the final run is Failed.
func MergeAttempts(first, final Observations) Observations {
	out := Observations{}
	out.Merge(final)
	for test, o := range first {
		if o == Passed {
			out.record(test, Passed)
			continue
		}
		if cur, ok := final[test]; ok && cur == Passed {
			out.record(test, PassedOnRetry)
		} else {
			out.record(test, Failed)
		}
	}
	return out
}

// Signals is the neutral retry-signal format. Any retry mechanism (the Go
// failed-package re-run script, a ZAP re-scan, ...) can emit it to
// flake-signals-*.json inside a flake-* artifact and appear in the ledger
// without a dedicated parser:
//
//	{"suite":"go/floor","tests":[{"test":"pkg.TestX","outcome":"passed_on_retry"}]}
type Signals struct {
	Suite string `json:"suite"`
	Tests []struct {
		Test    string `json:"test"`
		Outcome string `json:"outcome"`
	} `json:"tests"`
}

// ParseSignals reads a Signals document, returning its suite and outcomes.
func ParseSignals(r io.Reader) (string, Observations, error) {
	var s Signals
	if err := json.NewDecoder(r).Decode(&s); err != nil {
		return "", nil, fmt.Errorf("signals: %w", err)
	}
	if s.Suite == "" {
		return "", nil, fmt.Errorf("signals: missing suite")
	}
	obs := Observations{}
	for _, t := range s.Tests {
		o, ok := ParseOutcome(t.Outcome)
		if !ok || t.Test == "" {
			return "", nil, fmt.Errorf("signals: bad entry %q/%q", t.Test, t.Outcome)
		}
		obs.record(t.Test, o)
	}
	return s.Suite, obs, nil
}

type pwTest struct {
	ProjectName string `json:"projectName"`
	Status      string `json:"status"`
}

type pwSpec struct {
	Title string   `json:"title"`
	File  string   `json:"file"`
	Tests []pwTest `json:"tests"`
}

type pwSuite struct {
	Specs  []pwSpec  `json:"specs"`
	Suites []pwSuite `json:"suites"`
}

// ParsePlaywright reads Playwright's --reporter=json output. Per test:
// "flaky" = failed then passed; "unexpected" = failed every attempt;
// "expected" = passed first time; "skipped" is ignored.
func ParsePlaywright(r io.Reader) (Observations, error) {
	var root struct {
		Suites []pwSuite `json:"suites"`
	}
	if err := json.NewDecoder(r).Decode(&root); err != nil {
		return nil, fmt.Errorf("playwright: %w", err)
	}
	obs := Observations{}
	var walk func(s pwSuite)
	walk = func(s pwSuite) {
		for _, sp := range s.Specs {
			for _, t := range sp.Tests {
				id := sp.File + " > " + sp.Title
				if t.ProjectName != "" {
					id += " [" + t.ProjectName + "]"
				}
				switch t.Status {
				case "expected":
					obs.record(id, Passed)
				case "flaky":
					obs.record(id, PassedOnRetry)
				case "unexpected":
					obs.record(id, Failed)
				}
			}
		}
		for _, c := range s.Suites {
			walk(c)
		}
	}
	for _, s := range root.Suites {
		walk(s)
	}
	return obs, nil
}

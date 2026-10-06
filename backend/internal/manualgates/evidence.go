package manualgates

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// EvidenceDir is where the attestation script retains a run's JUnit XML. An
// attestation whose evidence is a path under it is machine-verifiable; any
// other evidence (a URL, a note) is accepted as a note but proves nothing to
// this package.
const EvidenceDir = ".github/manual-gates-evidence/"

// JUnitCounts sums tests / skipped / failures(+errors) over every <testsuite>
// element of a JUnit XML document, whether the root is a <testsuite> or a
// <testsuites> wrapper.
func JUnitCounts(data []byte) (tests, skipped, failures int, err error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	suites := 0
	for {
		tok, terr := dec.Token()
		if errors.Is(terr, io.EOF) {
			break
		}
		if terr != nil {
			return 0, 0, 0, terr
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "testsuite" {
			continue
		}
		suites++
		for _, a := range se.Attr {
			n, convErr := strconv.Atoi(a.Value)
			if convErr != nil {
				continue
			}
			switch a.Name.Local {
			case "tests":
				tests += n
			case "skipped":
				skipped += n
			case "failures", "errors":
				failures += n
			}
		}
	}
	if suites == 0 {
		return 0, 0, 0, errors.New("no <testsuite> element")
	}
	return tests, skipped, failures, nil
}

// VerifyEvidence checks every attestation whose evidence is a retained file
// under EvidenceDir: the file must exist, hash to junit_sha256, carry the
// gate's evidence_marker, and report the same counts the attestation claims --
// so the ledger cannot say "ran, 0 skipped" about an XML that says otherwise.
// read returns the file's bytes and whether it exists.
func VerifyEvidence(l Ledger, read func(path string) ([]byte, bool)) []string {
	var findings []string
	for _, g := range l.Gates {
		for j, a := range g.Attestations {
			if !strings.HasPrefix(a.Evidence, EvidenceDir) {
				continue
			}
			where := fmt.Sprintf("gate `%s` attestation #%d: evidence %s", g.ID, j+1, a.Evidence)
			data, ok := read(a.Evidence)
			if !ok {
				findings = append(findings, where+" does not exist")
				continue
			}
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != a.JUnitSHA256 {
				findings = append(findings, where+" does not hash to the recorded junit_sha256")
			}
			if g.EvidenceMarker != "" && !bytes.Contains(data, []byte(g.EvidenceMarker)) {
				findings = append(findings, fmt.Sprintf("%s never mentions %s -- not a run of the gate's test", where, g.EvidenceMarker))
			}
			tests, skipped, failures, err := JUnitCounts(data)
			switch {
			case err != nil:
				findings = append(findings, fmt.Sprintf("%s is not a JUnit XML report: %v", where, err))
			case tests != a.Tests || skipped != a.Skipped || failures != a.Failures:
				findings = append(findings, fmt.Sprintf("%s reports tests=%d skipped=%d failures=%d but the attestation claims tests=%d skipped=%d failures=%d", where, tests, skipped, failures, a.Tests, a.Skipped, a.Failures))
			}
		}
	}
	return findings
}

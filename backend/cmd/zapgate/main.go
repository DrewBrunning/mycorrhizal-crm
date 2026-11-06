// Command zapgate enforces the DAST gate (issue #368) on OWASP ZAP's
// traditional-JSON report.
//
// ZAP is a scanner, not a policy engine: it reports every alert, including
// false positives and known/accepted issues, and it has no notion of "this
// planted vulnerability must be present or the scan is blind". zapgate is the
// small, deterministic policy layer that turns ZAP's output into a pass/fail
// decision, in the same "ignore-list with justification" shape as
// android/.mobsf and docker/cis-hardening.ignore:
//
//  1. High/Medium alerts on the *canary* (the deliberately-vulnerable
//     dastcanary server) are expected and skipped — except that the presence
//     of the planted reflected-XSS (ZAP plugin 40012) on the canary is
//     *required*. If it is missing, the scan went blind and the gate fails
//     even when the app itself is clean.
//  2. High/Medium alerts on the *app* are failures unless an entry in the
//     ignore-list accepts them (plugin id + risk + URL regex), with the
//     justification logged so acceptances stay visible.
//     One narrow built-in acceptance exists (issue #2391): a plugin-10062 PII
//     Disclosure finding whose evidence is nothing but the last group of a
//     UUID in the response (a request ID echoed in the body), see acceptPIIUUID.
//  3. Anything else (Low/Info, non-high-risk) is ignored — the gate is on
//     High/Medium, matching the issue's ask.
//
// Configuration is via ZAPGATE_* env vars (see loadConfig); the defaults match
// the repo layout (zap/report.json, zap/dast.ignore, app on localhost:7300,
// canary on localhost:7301).
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// risk names, keyed by ZAP's riskcode string.
var riskNames = map[string]string{
	"0": "info",
	"1": "low",
	"2": "medium",
	"3": "high",
}

type config struct {
	reportPath   string
	ignorePath   string
	appHost      string
	canaryHost   string
	selfTestRule string
}

func defaultConfig() config {
	return config{
		reportPath:   "zap/report.json",
		ignorePath:   "zap/dast.ignore",
		appHost:      "localhost:7300",
		canaryHost:   "localhost:7301",
		selfTestRule: "40012",
	}
}

func loadConfig(getenv func(string) string) config {
	cfg := defaultConfig()
	if v := getenv("ZAPGATE_REPORT"); v != "" {
		cfg.reportPath = v
	}
	if v := getenv("ZAPGATE_IGNORE"); v != "" {
		cfg.ignorePath = v
	}
	if v := getenv("ZAPGATE_APP_HOST"); v != "" {
		cfg.appHost = v
	}
	if v := getenv("ZAPGATE_CANARY_HOST"); v != "" {
		cfg.canaryHost = v
	}
	if v := getenv("ZAPGATE_SELFTEST_RULE"); v != "" {
		cfg.selfTestRule = v
	}
	return cfg
}

// exitBlind is the process exit code when the ONLY unaccepted finding is the
// canary self-test (the scan went blind: the planted XSS was not seen). That is
// a probabilistic scanner/infra outcome, not a verdict on the app, so the
// release-battery wrapper (.github/scripts/zap-scan-gate.sh, issue #1487)
// re-runs the scan once on exactly this code. Any real app finding, with or
// without a blind self-test, exits 1 and is never retried -- a retry there
// would mask a genuine defect.
const exitBlind = 3

// errScanBlind marks a run whose sole failure is the canary self-test.
var errScanBlind = errors.New("scan blind")

// osExit is os.Exit through a seam so tests can drive main() itself.
var osExit = os.Exit

func main() {
	if err := run(os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "zapgate:", err)
		if errors.Is(err, errScanBlind) {
			osExit(exitBlind)
			return
		}
		osExit(1)
	}
}

// --- ZAP traditional-JSON report model ---

type zapReport struct {
	Site []zapSite `json:"site"`
}

type zapSite struct {
	Name   string     `json:"@name"`
	Alerts []zapAlert `json:"alerts"`
}

type zapAlert struct {
	PluginID  string        `json:"pluginid"`
	AlertRef  string        `json:"alertRef"`
	Alert     string        `json:"alert"`
	RiskCode  string        `json:"riskcode"`
	RiskDesc  string        `json:"riskdesc"`
	Instances []zapInstance `json:"instances"`
}

type zapInstance struct {
	URI string `json:"uri"`
	// Evidence and the response fields are filled by the traditional-json-plus
	// report template (zap/zap-dast.yaml); the plain template omits the
	// response. Only the plugin-10062 UUID check reads them.
	Evidence       string `json:"evidence"`
	ResponseHeader string `json:"responseheader"`
	ResponseBody   string `json:"responsebody"`
}

// --- ignore-list model ---

type ignoreRule struct {
	rule          string // ZAP pluginid or alertRef (e.g. "40045" or "10055-6"), or "*"
	risk          string // "high", "medium", or "*"
	urlRe         *regexp.Regexp
	justification string
}

func run(getenv func(string) string) error {
	cfg := loadConfig(getenv)

	report, err := readReport(cfg.reportPath)
	if err != nil {
		return fmt.Errorf("read report: %w", err)
	}
	rules, err := readIgnoreList(cfg.ignorePath)
	if err != nil {
		return fmt.Errorf("read ignore-list: %w", err)
	}

	var failures []string
	canarySelfTestSeen := false

	for _, site := range report.Site {
		isCanary := strings.Contains(site.Name, cfg.canaryHost)
		for _, alert := range site.Alerts {
			if !isGateRisk(alert.RiskCode) {
				continue
			}
			if isCanary {
				// Expected: the canary is vulnerable by design. Its planted
				// reflected-XSS is the self-test signal; everything else on the
				// canary is noise from an intentionally-bare server.
				if alert.PluginID == cfg.selfTestRule {
					canarySelfTestSeen = true
				}
				continue
			}
			if ok, reason := acceptPIIUUID(alert); ok {
				fmt.Printf("[ACCEPT] %s (%s) on %s — %s\n",
					alert.Alert, riskNames[alert.RiskCode], site.Name, reason)
				continue
			}
			if rule, ok := matchIgnore(rules, alert); ok {
				fmt.Printf("[ACCEPT] %s (%s) on %s — %s\n",
					alert.Alert, riskNames[alert.RiskCode], site.Name, rule.justification)
				continue
			}
			failures = append(failures, describe(alert, site.Name))
		}
	}

	if !canarySelfTestSeen {
		failures = append(failures, fmt.Sprintf(
			"self-test: no High/Medium alert for plugin %s found on the canary (%s) — the scan is blind or the canary was not scanned",
			cfg.selfTestRule, cfg.canaryHost))
	}

	if len(failures) > 0 {
		msg := fmt.Sprintf("%d unaccepted finding(s):\n  %s",
			len(failures), strings.Join(failures, "\n  "))
		// Only the self-test failed: no real app finding rode along.
		if len(failures) == 1 && !canarySelfTestSeen {
			return fmt.Errorf("%w: %s", errScanBlind, msg)
		}
		return errors.New(msg)
	}
	fmt.Println("OK: DAST gate passed — no unaccepted High/Medium findings, canary self-test present.")
	return nil
}

// isGateRisk reports whether an alert's riskcode is High or Medium.
func isGateRisk(riskcode string) bool {
	name, ok := riskNames[riskcode]
	if !ok {
		return false
	}
	return name == "high" || name == "medium"
}

// describe renders one failing alert for the final error, including the first
// instance URI so the triage message is actionable.
func describe(alert zapAlert, site string) string {
	uri := "(no instance)"
	if len(alert.Instances) > 0 {
		uri = alert.Instances[0].URI
	}
	return fmt.Sprintf("%s (%s) plugin=%s on %s: %s",
		alert.Alert, riskNames[alert.RiskCode], alert.PluginID, site, uri)
}

// matchIgnore returns the first ignore rule that accepts the alert. The rule
// key matches either the ZAP pluginid or the alertRef, so a rule can target a
// whole plugin ("40045") or a specific sub-finding ("10055-6").
func matchIgnore(rules []ignoreRule, alert zapAlert) (ignoreRule, bool) {
	risk := riskNames[alert.RiskCode]
	for _, r := range rules {
		if r.rule != "*" && r.rule != alert.PluginID && r.rule != alert.AlertRef {
			continue
		}
		if r.risk != "*" && r.risk != risk {
			continue
		}
		if r.urlRe == nil {
			return r, true
		}
		for _, inst := range alert.Instances {
			if r.urlRe.MatchString(inst.URI) {
				return r, true
			}
		}
	}
	return ignoreRule{}, false
}

func readReport(path string) (*zapReport, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is an operator-supplied report path, not request input
	if err != nil {
		return nil, err
	}
	var report zapReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &report, nil
}

// readIgnoreList parses zap/dast.ignore. Format: one rule per line,
// `<rule> <risk> <url-regex>` (whitespace-separated), where rule is a ZAP
// pluginid or alertRef (exact match) or `*`, risk is high/medium/* (case-
// insensitive), and url-regex is a Go regexp matched against instance URIs or
// `*`. A `#` begins a comment, on its own line or trailing a rule; trailing
// comments on a rule line are captured as the justification shown on [ACCEPT].
func readIgnoreList(path string) ([]ignoreRule, error) {
	f, err := os.Open(path) // #nosec G304 -- path is an operator-supplied ignore-list path, not request input
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only handle: Close cannot lose data

	var rules []ignoreRule
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()

		justification := ""
		if i := strings.Index(raw, "#"); i >= 0 {
			justification = strings.TrimSpace(raw[i+1:])
			raw = raw[:i]
		}
		fields := strings.Fields(raw)
		if len(fields) == 0 {
			continue // comment-only or blank line
		}
		if len(fields) < 3 {
			return nil, fmt.Errorf("%s:%d: expected '<rule> <risk> <url-regex>', got %q",
				path, lineNo, strings.TrimSpace(raw))
		}

		rule := fields[0]
		risk := strings.ToLower(fields[1])
		if risk != "*" && risk != "high" && risk != "medium" {
			return nil, fmt.Errorf("%s:%d: risk must be high, medium, or *, got %q",
				path, lineNo, fields[1])
		}

		var urlRe *regexp.Regexp
		if fields[2] != "*" {
			urlRe, err = regexp.Compile(fields[2])
			if err != nil {
				return nil, fmt.Errorf("%s:%d: bad url-regex %q: %w", path, lineNo, fields[2], err)
			}
		}

		rules = append(rules, ignoreRule{
			rule:          rule,
			risk:          risk,
			urlRe:         urlRe,
			justification: justification,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rules, nil
}

// piiPluginID is ZAP's PII Disclosure passive rule.
const piiPluginID = "10062"

// uuidRe matches a canonical UUID and captures its last (12-hex) group.
var uuidRe = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-([0-9a-f]{12})\b`)

// acceptPIIUUID accepts a plugin-10062 finding only when EVERY instance's
// evidence is purely the last group of a UUID in that instance's response and
// every occurrence of the evidence in the response is inside such a UUID. A
// random request ID whose last group is 12 decimal digits can pass ZAP's card
// regexes plus Luhn (issue #2391); a genuine free-standing number, or one that
// also appears outside a UUID, is not accepted. It fails closed when the
// response is absent from the report.
func acceptPIIUUID(alert zapAlert) (bool, string) {
	if alert.PluginID != piiPluginID || len(alert.Instances) == 0 {
		return false, ""
	}
	for _, inst := range alert.Instances {
		if !evidenceOnlyInUUIDs(inst.Evidence, inst.ResponseHeader+"\n"+inst.ResponseBody) {
			return false, ""
		}
	}
	return true, "false positive: the matched digits are only the last group of a UUID (request ID) in the response (issue #2391)"
}

func evidenceOnlyInUUIDs(evidence, text string) bool {
	if evidence == "" || text == "" {
		return false
	}
	total := strings.Count(text, evidence)
	if total == 0 {
		return false
	}
	inUUID := 0
	for _, m := range uuidRe.FindAllStringSubmatch(text, -1) {
		if m[1] == evidence {
			inUUID++
		}
	}
	return inUUID == total
}

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

// gateHighMedium are the risk names the gate enforces (issue #368: "gate on
// high/medium findings").
const gateHighMedium = "medium,high"

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

func main() {
	if err := run(os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "zapgate:", err)
		os.Exit(1)
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
		return fmt.Errorf("%d unaccepted finding(s):\n  %s",
			len(failures), strings.Join(failures, "\n  "))
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
	defer f.Close()

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

// Command dastcanary is a deliberately-vulnerable HTTP server that runs
// alongside the real app during the DAST (OWASP ZAP) scan only.
//
// Its entire job is to prove the scanner is actually detecting things
// (issue #368). If the scan ever goes blind — a broken proxy, a bad network
// mode, a ZAP rule disabled — the reflected-XSS canary below stops showing up
// in the report, and the zapgate self-test fails the workflow. A green DAST
// run is therefore "the scanner ran and found the things we planted", not just
// "the scanner ran".
//
// It never ships: dastcanary is not wired into the app, the all-in-one image,
// or any compose file. It is built and run only by
// .github/workflows/zap-dast.yml (and the local runbook in
// README-developer.md) against the throwaway test database.
//
// The two vulnerabilities planted here are deliberate and documented:
//
//   - GET /reflected?q=… reflects the q parameter into an HTML page without
//     escaping, the classic reflected-XSS shape that ZAP's active-scan rule
//     40012 ("Cross Site Scripting (Reflected)") detects deterministically.
//     This is the self-test signal zapgate asserts on.
//
//   - GET /idor/{id} returns a secret for any object id with no authorization
//     check. ZAP's default ruleset has no reliable IDOR detector (that is the
//     authorization-matrix E2E's job, issue #371), so this endpoint is not
//     asserted by the ZAP self-test — it exists so a future access-control
//     scan has a planted target, and so the Go tests below pin the intended
//     (vulnerable) behavior.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

const (
	// defaultAddr is where the canary listens; the workflow and runbook keep
	// it distinct from the app (7300) so ZAP can target both.
	defaultAddr = ":7301"
	// canaryIDPrefix is prepended to the IDOR secret so a test (or a future
	// scanner) can tell a canary hit apart from anything real.
	canaryIDPrefix = "canary-secret-for-id-"
)

func main() {
	addr := addrFromEnv(os.Getenv)

	srv := &http.Server{Addr: addr, Handler: newMux()}
	log.Printf("dastcanary listening on %s (vulnerable-by-design, DAST-only)", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("dastcanary: %v", err)
	}
}

// addrFromEnv returns the listen address, honoring DAST_CANARY_ADDR (injected
// so the default is testable without mutating real process env).
func addrFromEnv(getenv func(string) string) string {
	if v := getenv("DAST_CANARY_ADDR"); v != "" {
		return v
	}
	return defaultAddr
}

// newMux wires the three canary routes. Split out of main so the handlers are
// testable via httptest without binding a real socket.
func newMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/reflected", reflectedHandler)
	mux.HandleFunc("/idor/", idorHandler)
	return mux
}

// healthHandler lets the workflow (and a human) wait for the canary to be up
// before the scan starts.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "ok")
}

// reflectedHandler reflects the q query parameter straight into an HTML page.
// This is a reflected-XSS sink by design: no HTML escaping, no CSP, no
// nosniff. The response is minimal on purpose so ZAP's rule 40012 sees the
// reflected payload in a plain text/html body.
func reflectedHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		q = "hello"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Intentional: raw (unescaped) interpolation of user input — the planted
	// vulnerability. Do not "fix" this; it is the whole point of the canary
	// (ZAP must detect it for the DAST self-test). Suppressed so CodeQL stops
	// flagging the deliberate sink.
	// lgtm[go/reflected-xss]
	fmt.Fprintf(w, "<html><body><h1>Reflected</h1><p>%s</p></body></html>", q)
}

// idorHandler returns a secret for whatever object id is in the path, with no
// authentication or ownership check. The planted IDOR vulnerability.
func idorHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/idor/")
	if id == "" {
		id = "missing"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintf(w, `{"object":"user","id":%q,"secret":%q}`, id, canaryIDPrefix+id)
}

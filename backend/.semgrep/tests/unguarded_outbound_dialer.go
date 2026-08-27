// Hand-verification snippets for the mycorrhizal-unguarded-outbound-dialer
// rule (issue #609). See the verification command in
// .semgrep/mycorrhizal-traps.yaml's header comment.
//
// The rule exists because an eighth integration client that does not route its
// dialer through httputil.SafeDialContext is a silent SSRF regression: the
// public-IP-only dialer (httputil/safedial.go) is where DNS rebinding and
// redirect-to-internal-address attacks are closed, because every connection a
// transport opens passes through DialContext. These snippets mirror the shapes
// the seven existing correct clients use (inlined in an http.Client literal,
// bound to a local from SafeDialContext, wired by field mutation on a shared
// transport) and the shapes a regression would take.
package tests

// ruleid: mycorrhizal-unguarded-outbound-dialer
func inlinedClientTransportWithNoDialer() {
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			IdleConnTimeout: 30 * time.Second,
		},
	}
	_ = client
}

// ruleid: mycorrhizal-unguarded-outbound-dialer
func inlinedClientTransportWithRawNetDialer() {
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		},
	}
	_ = client
}

// ruleid: mycorrhizal-unguarded-outbound-dialer
func standaloneTransportWithRawNetDialer() {
	transport := &http.Transport{
		DialContext: (&net.Dialer{}).DialContext,
	}
	_ = transport
}

// ruleid: mycorrhizal-unguarded-outbound-dialer
func standaloneTransportWithFuncLiteralDialer() {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			d := &net.Dialer{}
			return d.DialContext(ctx, network, addr)
		},
	}
	_ = transport
}

// ruleid: mycorrhizal-unguarded-outbound-dialer
func standaloneTransportWithCallResultDialer() {
	transport := &http.Transport{
		DialContext: makeRawDialer(),
	}
	_ = transport
}

// ok: mycorrhizal-unguarded-outbound-dialer
func inlinedClientTransportWithSafeDialer() {
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialContext: httputil.SafeDialContext(unreachableErr, privateErr),
		},
	}
	_ = client
}

// ok: mycorrhizal-unguarded-outbound-dialer
func inlinedClientTransportWithSafeDialerLocal() {
	safeDialContext := httputil.SafeDialContext(unreachableErr, privateErr)
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialContext: safeDialContext,
		},
	}
	_ = client
}

// ok: mycorrhizal-unguarded-outbound-dialer
func sharedTransportWiredByMutation(transport *http.Transport) {
	transport.DialContext = privateBlockingDialContext
}

// ok: mycorrhizal-unguarded-outbound-dialer
func transportConstructor() *http.Transport {
	return &http.Transport{
		TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),

		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConnsPerHost:   4,
	}
}

// ok: mycorrhizal-unguarded-outbound-dialer
func bareClientWithNoTransport() {
	client := &http.Client{Timeout: 5 * time.Second}
	_ = client
}

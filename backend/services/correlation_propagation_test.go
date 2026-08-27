package services

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"mycorrhizal/logger"
)

type headerCapturingTransport struct{ got http.Header }

func (h *headerCapturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	h.got = req.Header.Clone()
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}, nil
}

// TestOutboundSyncCarriesCorrelationID proves the contact/calendar sync round
// trippers copy the caller's correlation ID onto the outbound request as
// X-Correlation-ID, so a remote's access log ties back to the triggering UI
// action or scheduled run (issue #425).
func TestOutboundSyncCarriesCorrelationID(t *testing.T) {
	cases := map[string]http.RoundTripper{
		"contact":  &contactRoundTripper{},
		"calendar": &calendarRoundTripper{},
	}
	for name, rt := range cases {
		t.Run(name, func(t *testing.T) {
			cap := &headerCapturingTransport{}
			switch v := rt.(type) {
			case *contactRoundTripper:
				v.base = cap
			case *calendarRoundTripper:
				v.base = cap
			}

			ctx := logger.WithCorrelationID(context.Background(), "corr-abc")
			req := httptest.NewRequest(http.MethodGet, "https://remote.example/dav", nil).WithContext(ctx)

			_, err := rt.RoundTrip(req)
			require.NoError(t, err)
			require.Equal(t, "corr-abc", cap.got.Get("X-Correlation-ID"))
		})
	}
}

func TestOutboundSyncNoCorrelationIDWhenAbsent(t *testing.T) {
	cap := &headerCapturingTransport{}
	rt := &contactRoundTripper{base: cap}
	req := httptest.NewRequest(http.MethodGet, "https://remote.example/dav", nil)

	_, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Empty(t, cap.got.Get("X-Correlation-ID"))
}

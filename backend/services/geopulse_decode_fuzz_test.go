package services

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// geoPulseFuzzBody is a body that records whether it was closed, so the fuzz
// target can assert decodeGeoPulseData always releases the response body.
type geoPulseFuzzBody struct {
	r      io.Reader
	closed bool
}

func (b *geoPulseFuzzBody) Read(p []byte) (int, error) { return b.r.Read(p) }
func (b *geoPulseFuzzBody) Close() error               { b.closed = true; return nil }

// FuzzDecodeGeoPulseData fuzzes decodeGeoPulseData (issue #1626), which
// unwraps the {status, message, data} envelope of a response from a
// *remote GeoPulse server* — input this instance does not control. The
// property: it never panics, always closes the body, and the envelope's own
// rules are enforced — a non-"success" status, a missing/JSON-null data
// member, or bytes that are not a JSON envelope are all errors, never a
// silent success.
func FuzzDecodeGeoPulseData(f *testing.F) {
	seeds := [][]byte{
		[]byte(`{"status":"success","data":{"userId":"u1"}}`),
		[]byte(`{"status":"success","data":{"stays":[]}}`),
		[]byte(`{"status":"success","data":null}`),
		[]byte(`{"status":"error","message":"nope","data":{}}`),
		[]byte(`{"status":"success"}`),
		[]byte(`{"status":"success","data":"a string"}`),
		[]byte(`[]`),
		[]byte(`null`),
		[]byte(`not json`),
		[]byte(``),
		[]byte(`{"status":"success","data":123}`),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, body []byte) {
		rc := &geoPulseFuzzBody{r: bytes.NewReader(body)}
		resp := &http.Response{StatusCode: http.StatusOK, Body: rc}

		var out struct {
			UserID string `json:"userId"`
		}
		err := decodeGeoPulseData(resp, &out)
		if !rc.closed {
			t.Fatalf("decodeGeoPulseData did not close the response body for %q", body)
		}

		// Independent envelope probe: the rules the doc comment promises.
		var env struct {
			Status string          `json:"status"`
			Data   json.RawMessage `json:"data"`
		}
		if json.Unmarshal(body, &env) != nil {
			if err == nil {
				t.Fatalf("decodeGeoPulseData accepted non-JSON body %q", body)
			}
			return
		}
		if env.Status != "success" && err == nil {
			t.Fatalf("decodeGeoPulseData accepted envelope status %q", env.Status)
		}
		if env.Status == "success" && (len(env.Data) == 0 || string(env.Data) == "null") && err == nil {
			t.Fatalf("decodeGeoPulseData accepted a success envelope with no data")
		}
	})
}

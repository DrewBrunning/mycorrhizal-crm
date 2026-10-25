package services

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// go-webdav v0.7.0 reads ETags with strconv.Unquote, which only accepts the
// strong quoted form ("abc"). Servers in the wild also send the bare form
// (abc) and the weak form (W/"abc"); either one fails the whole sync with
// "webdav: failed to unquote ETag" (emersion/go-webdav#118, #165; issue #2003).
//
// The transport wrapper below rewrites every response ETag — the ETag header
// and every <DAV:getetag> element in a multistatus body — into the quoted form
// the library expects. Our own CardDAV server's emitted ETags are untouched;
// this only affects responses consumed by the CardDAV/CalDAV sync clients.
//
// The ETag is only a stored transport token for us: change detection uses the
// content hash (see contactContentHash), so dropping a weak marker does not
// weaken conflict detection.

// normalizeETagValue rewrites an ETag value (header or XML text, already
// entity-decoded) into the strong quoted form accepted by strconv.Unquote.
// An already-quoted value is not double-quoted; strconv.Quote also makes any
// embedded backslash or quote safe for Unquote.
func normalizeETagValue(raw string) string {
	value := strings.TrimSpace(raw)
	if len(value) >= 2 && (value[0] == 'W' || value[0] == 'w') && value[1] == '/' {
		value = strings.TrimSpace(value[2:])
	}
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}
	return strconv.Quote(value)
}

// normalizeETagResponse normalizes the ETag header and, for XML responses
// (REPORT/multistatus and anything served as XML), the getetag elements in
// the body. It reports whether it buffered and replaced the body, so callers
// know the size limit has already been enforced and do not wrap it again.
func normalizeETagResponse(req *http.Request, resp *http.Response, maxBodyBytes int64, tooLarge error) (bool, error) {
	if etag := resp.Header.Get("ETag"); etag != "" {
		resp.Header.Set("ETag", normalizeETagValue(etag))
	}
	if resp.Body == nil || !responseBodyMayCarryETags(req, resp) {
		return false, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return false, err
	}
	if int64(len(body)) > maxBodyBytes {
		return false, tooLarge
	}
	if err := resp.Body.Close(); err != nil {
		return false, err
	}

	normalized := normalizeGetETagXML(body)
	resp.Body = io.NopCloser(bytes.NewReader(normalized))
	resp.ContentLength = int64(len(normalized))
	if resp.Header.Get("Content-Length") != "" {
		resp.Header.Set("Content-Length", strconv.Itoa(len(normalized)))
	}
	return true, nil
}

// responseBodyMayCarryETags is true for the two shapes that can carry a
// DAV:getetag property: a REPORT response (CardDAV/CalDAV multistatus) and an
// XML body. Plain GETs (e.g. the ICS fallback) stay streaming.
func responseBodyMayCarryETags(req *http.Request, resp *http.Response) bool {
	if req != nil && strings.EqualFold(req.Method, "REPORT") {
		return true
	}
	return strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "xml")
}

// normalizeGetETagXML rewrites the character data of every getetag element
// while leaving all other bytes (including entity escaping in address-data
// and calendar-data) untouched. It parses with encoding/xml to locate the
// text ranges, then splices replacements back into the original body; an
// unparseable body is returned unchanged so the library's own error surfaces.
func normalizeGetETagXML(body []byte) []byte {
	if !bytes.Contains(body, []byte("getetag")) {
		return body
	}

	type edit struct {
		start int
		end   int
		text  []byte
	}
	var edits []edit

	dec := xml.NewDecoder(bytes.NewReader(body))
	depth := 0
	for {
		start := dec.InputOffset()
		tok, err := dec.Token()
		end := dec.InputOffset()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return body
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "getetag" {
				depth++
			}
		case xml.EndElement:
			if t.Name.Local == "getetag" && depth > 0 {
				depth--
			}
		case xml.CharData:
			if depth > 0 {
				text := escapeXMLText(normalizeETagValue(string(t)))
				edits = append(edits, edit{start: int(start), end: int(end), text: text})
			}
		}
	}
	if len(edits) == 0 {
		return body
	}

	out := make([]byte, 0, len(body)+len(edits)*4)
	last := 0
	for _, e := range edits {
		if e.start < last || e.end < e.start || e.end > len(body) {
			return body
		}
		out = append(out, body[last:e.start]...)
		out = append(out, e.text...)
		last = e.end
	}
	return append(out, body[last:]...)
}

func escapeXMLText(s string) []byte {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.Bytes()
}

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
// The transport wrapper below rewrites response ETags — the ETag header and
// every <DAV:getetag> element in a multistatus body, including empty and
// self-closing elements — into the quoted form the library expects. Our own
// CardDAV server's emitted ETags are untouched; this only affects responses
// consumed by the CardDAV/CalDAV sync clients.
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
// and calendar-data) untouched. An empty text element (<getetag/> or
// <getetag></getetag>) has no CharData token to rewrite, so it is expanded to
// a quoted empty value here; otherwise go-webdav's strconv.Unquote("") fails
// the sync. It parses with encoding/xml to locate the text ranges, then
// splices replacements back into the original body; an unparseable body is
// returned unchanged so the library's own error surfaces.
func normalizeGetETagXML(body []byte) []byte {
	if !bytes.Contains(body, []byte("getetag")) {
		return body
	}

	type edit struct {
		start int
		end   int
		text  []byte
	}
	type openETag struct {
		start    int
		startEnd int
		name     string
		sawText  bool
	}
	var edits []edit
	var open []openETag

	dec := xml.NewDecoder(bytes.NewReader(body))
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
				open = append(open, openETag{
					start:    int(start),
					startEnd: int(end),
					name:     xmlElementName(body[start:end]),
				})
			}
		case xml.EndElement:
			if t.Name.Local != "getetag" || len(open) == 0 {
				continue
			}
			el := open[len(open)-1]
			open = open[:len(open)-1]
			if el.sawText {
				continue
			}
			text := escapeXMLText(normalizeETagValue(""))
			if int(start) == int(end) {
				// Self-closing element: there is no content range to insert
				// into, so rewrite the start tag as a paired element.
				edits = append(edits, edit{
					start: el.start,
					end:   el.startEnd,
					text:  expandEmptyGetETag(body[el.start:el.startEnd], el.name, text),
				})
				continue
			}
			edits = append(edits, edit{start: int(start), end: int(start), text: text})
		case xml.CharData:
			for i := range open {
				open[i].sawText = true
			}
			if len(open) > 0 {
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

// xmlElementName extracts the lexical qualified name from a raw start tag.
func xmlElementName(startTag []byte) string {
	i := 1
	for i < len(startTag) && isXMLSpace(startTag[i]) {
		i++
	}
	j := i
	for j < len(startTag) && !isXMLSpace(startTag[j]) && startTag[j] != '/' && startTag[j] != '>' {
		j++
	}
	return string(startTag[i:j])
}

func isXMLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

// expandEmptyGetETag turns a self-closing getetag start tag into a paired
// element carrying text, preserving its name and any attributes.
func expandEmptyGetETag(startTag []byte, name string, text []byte) []byte {
	base := bytes.TrimSuffix(startTag, []byte("/>"))
	out := make([]byte, 0, len(base)+len(text)+len(name)+3)
	out = append(out, base...)
	out = append(out, '>')
	out = append(out, text...)
	out = append(out, '<', '/')
	out = append(out, name...)
	out = append(out, '>')
	return out
}

func escapeXMLText(s string) []byte {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.Bytes()
}

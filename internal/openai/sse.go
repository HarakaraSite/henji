package openai

import (
	"bufio"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/openai/openai-go/option"
)

// ignoreEmptySSEEvents works around openai-go v1's decoder, which yields an
// event for the blank delimiter after an SSE comment. Such an event has no
// data, but the SDK subsequently attempts to decode it as JSON.
//
// Keep all non-empty SSE lines unchanged. Only the delimiter of a block that
// contains no data line is removed, so completion chunks, [DONE], and API
// error payloads retain the SDK's normal handling.
func ignoreEmptySSEEvents(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	resp, err := next(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}

	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err == nil && strings.EqualFold(contentType, "text/event-stream") {
		resp.Body = &emptySSEEventFilter{
			body:   resp.Body,
			reader: bufio.NewReader(resp.Body),
		}
	}
	return resp, nil
}

// emptySSEEventFilter is a streaming filter: it retains no full event body
// and preserves HTTP response ownership through Close.
type emptySSEEventFilter struct {
	body    io.ReadCloser
	reader  *bufio.Reader
	pending []byte
	hasData bool
	err     error
}

func (f *emptySSEEventFilter) Read(p []byte) (int, error) {
	for len(f.pending) == 0 && f.err == nil {
		line, err := f.reader.ReadBytes('\n')
		if len(line) > 0 {
			if sseBlankLine(line) {
				if f.hasData {
					f.pending = line
				}
				f.hasData = false
			} else {
				if sseDataLine(line) {
					f.hasData = true
				}
				f.pending = line
			}
		}
		if err != nil {
			f.err = err
		}
	}

	if len(f.pending) == 0 {
		return 0, f.err
	}
	n := copy(p, f.pending)
	f.pending = f.pending[n:]
	return n, nil
}

func (f *emptySSEEventFilter) Close() error {
	return f.body.Close()
}

func sseBlankLine(line []byte) bool {
	return len(bytesTrimLineEnding(line)) == 0
}

func sseDataLine(line []byte) bool {
	return strings.HasPrefix(string(bytesTrimLineEnding(line)), "data:")
}

func bytesTrimLineEnding(line []byte) []byte {
	return []byte(strings.TrimSuffix(strings.TrimSuffix(string(line), "\n"), "\r"))
}

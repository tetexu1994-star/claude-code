package lsp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
)

// Transport implements JSON-RPC wire protocol with Content-Length framing.
// LSP uses HTTP-style headers: Content-Length: <N>\r\n\r\n<body>
type Transport struct {
	reader *bufio.Reader
	writer io.Writer
}

// NewTransport creates a new LSP transport over the given read/write streams.
func NewTransport(r io.Reader, w io.Writer) *Transport {
	return &Transport{
		reader: bufio.NewReader(r),
		writer: w,
	}
}

// ReadMessage reads a framed JSON-RPC message from the reader.
// Format: Content-Length: <N>\r\n\r\n<JSON body of N bytes>
func (t *Transport) ReadMessage() ([]byte, error) {
	var contentLength int
	for {
		line, err := t.reader.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("transport read header: %w", err)
		}

		line = bytes.TrimRight(line, "\r\n")
		if len(line) == 0 {
			// Empty line marks end of headers
			break
		}

		if bytes.HasPrefix(line, []byte("Content-Length: ")) {
			n, err := strconv.Atoi(string(line[len("Content-Length: "):]))
			if err != nil {
				return nil, fmt.Errorf("transport parse Content-Length: %w", err)
			}
			contentLength = n
		}
		// Ignore other headers (Content-Type, etc.)
	}

	if contentLength <= 0 {
		return nil, fmt.Errorf("transport read: missing or invalid Content-Length header")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(t.reader, body); err != nil {
		return nil, fmt.Errorf("transport read body: %w", err)
	}

	return body, nil
}

// WriteMessage writes a framed JSON-RPC message to the writer.
// Format: Content-Length: <N>\r\n\r\n<JSON body>
func (t *Transport) WriteMessage(data []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := t.writer.Write([]byte(header)); err != nil {
		return fmt.Errorf("transport write header: %w", err)
	}
	if _, err := t.writer.Write(data); err != nil {
		return fmt.Errorf("transport write body: %w", err)
	}
	return nil
}

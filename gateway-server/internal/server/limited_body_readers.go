package server

import (
	"fmt"
	"io"
)

type limitedBodyReadCloser struct {
	io.ReadCloser
	limit int64
	read  int64
}

func newLimitedBodyReadCloser(rc io.ReadCloser, limit int64) io.ReadCloser {
	if rc == nil {
		return httpNoBodyReadCloser{}
	}
	if limit <= 0 {
		return rc
	}
	return &limitedBodyReadCloser{ReadCloser: rc, limit: limit}
}

func (l *limitedBodyReadCloser) Read(p []byte) (int, error) {
	n, err := l.ReadCloser.Read(p)
	l.read += int64(n)
	if l.read > l.limit {
		return n, fmt.Errorf("request body exceeds max_request_body_bytes=%d", l.limit)
	}
	return n, err
}

type httpNoBodyReadCloser struct{}

func (httpNoBodyReadCloser) Read([]byte) (int, error) { return 0, io.EOF }
func (httpNoBodyReadCloser) Close() error             { return nil }

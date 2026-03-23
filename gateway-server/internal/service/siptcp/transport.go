package siptcp

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	headerTerminator = "\r\n\r\n"
	contentLengthKey = "content-length"
	maxHeaderBytes   = 16 * 1024
)

var (
	ErrInvalidFrame      = errors.New("invalid sip tcp frame")
	ErrMessageTooLarge   = errors.New("sip tcp message exceeds max size")
	ErrMissingBodyLength = errors.New("sip tcp frame missing content-length")
)

type Framer struct {
	maxMessageBytes int
	buffer          []byte
}

func NewFramer(maxMessageBytes int) *Framer {
	return &Framer{maxMessageBytes: maxMessageBytes}
}

func Encode(payload []byte) []byte {
	headers := fmt.Sprintf("SIP-TUNNEL/1.0\r\nContent-Length: %d\r\n\r\n", len(payload))
	return append([]byte(headers), payload...)
}

func (f *Framer) Feed(chunk []byte) ([][]byte, error) {
	if len(chunk) > 0 {
		f.buffer = append(f.buffer, chunk...)
	}
	frames := make([][]byte, 0)
	for {
		headerEnd := bytes.Index(f.buffer, []byte(headerTerminator))
		if headerEnd < 0 {
			if len(f.buffer) > maxHeaderBytes {
				return nil, ErrInvalidFrame
			}
			return frames, nil
		}
		headerBlock := string(f.buffer[:headerEnd])
		contentLen, err := parseContentLength(headerBlock)
		if err != nil {
			return nil, err
		}
		if contentLen > f.maxMessageBytes {
			return nil, ErrMessageTooLarge
		}
		total := headerEnd + len(headerTerminator) + contentLen
		if total > len(f.buffer) {
			if len(f.buffer) > f.maxMessageBytes+maxHeaderBytes {
				return nil, ErrMessageTooLarge
			}
			return frames, nil
		}
		payload := make([]byte, contentLen)
		copy(payload, f.buffer[headerEnd+len(headerTerminator):total])
		frames = append(frames, payload)
		f.buffer = f.buffer[total:]
	}
}

func parseContentLength(headerBlock string) (int, error) {
	lines := strings.Split(headerBlock, "\r\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(parts[0]), contentLengthKey) {
			n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil || n < 0 {
				return 0, fmt.Errorf("%w: bad content-length", ErrInvalidFrame)
			}
			return n, nil
		}
	}
	return 0, ErrMissingBodyLength
}

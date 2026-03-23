package siptext

import (
	"bufio"
	"bytes"
	"fmt"
	"net/textproto"
	"sort"
	"strconv"
	"strings"
)

const Version = "SIP/2.0"

type Message struct {
	IsRequest    bool
	Method       string
	RequestURI   string
	StatusCode   int
	ReasonPhrase string
	Version      string
	Headers      textproto.MIMEHeader
	Body         []byte
}

func NewRequest(method, requestURI string) *Message {
	return &Message{IsRequest: true, Method: strings.ToUpper(strings.TrimSpace(method)), RequestURI: strings.TrimSpace(requestURI), Version: Version, Headers: make(textproto.MIMEHeader)}
}

func NewResponse(req *Message, statusCode int, reason string) *Message {
	m := &Message{IsRequest: false, StatusCode: statusCode, ReasonPhrase: strings.TrimSpace(reason), Version: Version, Headers: make(textproto.MIMEHeader)}
	if m.ReasonPhrase == "" {
		m.ReasonPhrase = defaultReason(statusCode)
	}
	if req != nil {
		for _, h := range []string{"Via", "From", "To", "Call-ID", "CSeq", "Subject"} {
			if vals, ok := req.Headers[textproto.CanonicalMIMEHeaderKey(h)]; ok {
				m.Headers[textproto.CanonicalMIMEHeaderKey(h)] = append([]string(nil), vals...)
			}
		}
	}
	return m
}

func Parse(raw []byte) (*Message, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty sip message")
	}
	headerEnd := bytes.Index(raw, []byte("\r\n\r\n"))
	if headerEnd < 0 {
		return nil, fmt.Errorf("sip message missing CRLFCRLF")
	}
	bodyStart := headerEnd + 4
	reader := textproto.NewReader(bufio.NewReader(bytes.NewReader(raw[:bodyStart])))
	startLine, err := reader.ReadLine()
	if err != nil {
		return nil, fmt.Errorf("read start line: %w", err)
	}
	headers, err := reader.ReadMIMEHeader()
	if err != nil {
		return nil, fmt.Errorf("read headers: %w", err)
	}
	contentLength := 0
	if rawLen := strings.TrimSpace(headers.Get("Content-Length")); rawLen != "" {
		v, err := strconv.Atoi(rawLen)
		if err != nil || v < 0 {
			return nil, fmt.Errorf("invalid content-length: %q", rawLen)
		}
		contentLength = v
	}
	if contentLength > len(raw)-bodyStart {
		return nil, fmt.Errorf("sip body truncated: need=%d have=%d", contentLength, len(raw)-bodyStart)
	}
	msg := &Message{Headers: make(textproto.MIMEHeader), Body: append([]byte(nil), raw[bodyStart:bodyStart+contentLength]...)}
	for k, vals := range headers {
		msg.Headers[k] = append([]string(nil), vals...)
	}
	parts := strings.SplitN(strings.TrimSpace(startLine), " ", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid sip start line: %q", startLine)
	}
	if strings.EqualFold(parts[0], Version) {
		if len(parts) < 3 {
			return nil, fmt.Errorf("invalid sip status line: %q", startLine)
		}
		code, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid status code: %q", parts[1])
		}
		msg.IsRequest = false
		msg.Version = Version
		msg.StatusCode = code
		msg.ReasonPhrase = strings.TrimSpace(parts[2])
		return msg, nil
	}
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid sip request line: %q", startLine)
	}
	if !strings.EqualFold(strings.TrimSpace(parts[2]), Version) {
		return nil, fmt.Errorf("unsupported sip version: %q", parts[2])
	}
	msg.IsRequest = true
	msg.Method = strings.ToUpper(strings.TrimSpace(parts[0]))
	msg.RequestURI = strings.TrimSpace(parts[1])
	msg.Version = Version
	return msg, nil
}

func (m *Message) SetHeader(name, value string) {
	if m.Headers == nil {
		m.Headers = make(textproto.MIMEHeader)
	}
	m.Headers.Set(name, strings.TrimSpace(value))
}

func (m *Message) Header(name string) string {
	if m == nil || m.Headers == nil {
		return ""
	}
	return strings.TrimSpace(m.Headers.Get(name))
}

func (m *Message) Bytes() []byte {
	if m == nil {
		return nil
	}
	if m.Headers == nil {
		m.Headers = make(textproto.MIMEHeader)
	}
	version := strings.TrimSpace(m.Version)
	if version == "" {
		version = Version
	}
	m.Headers.Set("Content-Length", strconv.Itoa(len(m.Body)))
	keys := make([]string, 0, len(m.Headers))
	for k := range m.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	if m.IsRequest {
		fmt.Fprintf(&buf, "%s %s %s\r\n", strings.ToUpper(strings.TrimSpace(m.Method)), strings.TrimSpace(m.RequestURI), version)
	} else {
		reason := strings.TrimSpace(m.ReasonPhrase)
		if reason == "" {
			reason = defaultReason(m.StatusCode)
		}
		fmt.Fprintf(&buf, "%s %d %s\r\n", version, m.StatusCode, reason)
	}
	for _, k := range keys {
		for _, v := range m.Headers[k] {
			fmt.Fprintf(&buf, "%s: %s\r\n", k, v)
		}
	}
	buf.WriteString("\r\n")
	buf.Write(m.Body)
	return buf.Bytes()
}

func defaultReason(code int) string {
	switch code {
	case 100:
		return "Trying"
	case 180:
		return "Ringing"
	case 200:
		return "OK"
	case 400:
		return "Bad Request"
	case 401:
		return "Unauthorized"
	case 404:
		return "Not Found"
	case 408:
		return "Request Timeout"
	case 481:
		return "Call/Transaction Does Not Exist"
	case 500:
		return "Server Internal Error"
	case 501:
		return "Not Implemented"
	default:
		return "OK"
	}
}

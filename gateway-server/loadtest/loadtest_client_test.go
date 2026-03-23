package loadtest

import "testing"

func TestClassifyErrRecognizesLocalAddrExhausted(t *testing.T) {
	errType := classifyErr(assertClientErr(`Post "http://127.0.0.1:28080/healthz": dial tcp 127.0.0.1:28080: connectex: Only one usage of each socket address (protocol/network address/port) is normally permitted.`))
	if errType != "local_addr_exhausted" {
		t.Fatalf("unexpected err type: %s", errType)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func assertClientErr(s string) error { return errString(s) }

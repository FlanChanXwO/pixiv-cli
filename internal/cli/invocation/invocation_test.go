package invocation

import (
	"bytes"
	"testing"
)

func TestNewStreamsPreservesProvidedStreams(t *testing.T) {
	in := bytes.NewBufferString("input")
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}

	streams := NewStreams(in, out, errOut)

	if streams.In != in || streams.Out != out || streams.Err != errOut {
		t.Fatalf("NewStreams did not preserve caller streams: %#v", streams)
	}
}

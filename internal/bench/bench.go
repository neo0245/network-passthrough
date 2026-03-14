package bench

import (
	"bytes"
	"time"

	"slpp/internal/core"
)

type Result struct {
	Name      string        `json:"name"`
	Duration  time.Duration `json:"duration"`
	Bytes     int           `json:"bytes"`
	Iterations int          `json:"iterations"`
}

func CodecEncode(iterations int) Result {
	codec := core.Codec{}
	frame := core.Frame{Type: core.FrameTCPDATA, Payload: bytes.Repeat([]byte("a"), 1024)}
	start := time.Now()
	for i := 0; i < iterations; i++ {
		var buf bytes.Buffer
		_ = codec.Encode(frame, &buf)
	}
	return Result{Name: "codec_encode", Duration: time.Since(start), Bytes: 1024, Iterations: iterations}
}

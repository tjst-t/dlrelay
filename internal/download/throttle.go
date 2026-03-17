package download

import (
	"context"
	"io"

	"golang.org/x/time/rate"
)

// ThrottledReader wraps an io.Reader with bandwidth limiting.
type ThrottledReader struct {
	r       io.Reader
	limiter *rate.Limiter
	ctx     context.Context
}

// NewThrottledReader creates a reader limited to bytesPerSec throughput.
// If bytesPerSec is 0 or negative, no throttling is applied.
func NewThrottledReader(ctx context.Context, r io.Reader, bytesPerSec int64) io.Reader {
	if bytesPerSec <= 0 {
		return r
	}
	burst := int(bytesPerSec)
	if burst > 1024*1024 {
		burst = 1024 * 1024 // cap burst at 1MB
	}
	if burst < 1024 {
		burst = 1024
	}
	return &ThrottledReader{
		r:       r,
		limiter: rate.NewLimiter(rate.Limit(bytesPerSec), burst),
		ctx:     ctx,
	}
}

func (t *ThrottledReader) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if n > 0 {
		if waitErr := t.limiter.WaitN(t.ctx, n); waitErr != nil {
			return n, waitErr
		}
	}
	return n, err
}

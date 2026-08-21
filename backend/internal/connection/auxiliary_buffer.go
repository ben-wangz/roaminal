package connection

import (
	"bytes"
	"io"
)

type cappedBuffer struct {
	data     bytes.Buffer
	limit    int
	overflow bool
}

func (b *cappedBuffer) Write(value []byte) (int, error) {
	if b.data.Len()+len(value) > b.limit {
		remaining := b.limit - b.data.Len()
		if remaining > 0 {
			_, _ = b.data.Write(value[:remaining])
		}
		b.overflow = true
		return len(value), nil
	}
	return b.data.Write(value)
}

var _ io.Writer = (*cappedBuffer)(nil)

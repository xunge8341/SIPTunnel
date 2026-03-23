package server

import "sync"

const pooledCopyBufferSize = 32 * 1024

var pooledCopyBuffer = sync.Pool{
	New: func() any {
		buf := make([]byte, pooledCopyBufferSize)
		return &buf
	},
}

func acquireCopyBuffer() []byte {
	return *pooledCopyBuffer.Get().(*[]byte)
}

func releaseCopyBuffer(buf []byte) {
	if cap(buf) < pooledCopyBufferSize {
		return
	}
	buf = buf[:pooledCopyBufferSize]
	pooledCopyBuffer.Put(&buf)
}

package reader

import "sync"

type buffer struct {
	buf    []byte
	offset int
}

func (b *buffer) isEmpty() bool {
	return b == nil || len(b.buf)-b.offset <= 0
}

func (b *buffer) buffer() []byte {
	return b.buf[b.offset:]
}

func (b *buffer) increment(n int) {
	b.offset += n
}

// reset prepares buffer for reuse
func (b *buffer) reset() {
	b.buf = b.buf[:0]
	b.offset = 0
}

// Global buffer pool declared in tg_reader.go, accessed here
var bufferPoolPtr *sync.Pool

func getBuffer() *buffer {
	if bufferPoolPtr == nil {
		return &buffer{buf: make([]byte, 0, 4*1024*1024)}
	}
	b := bufferPoolPtr.Get().(*buffer)
	b.reset()
	return b
}

func putBuffer(b *buffer) {
	if bufferPoolPtr == nil || b == nil {
		return
	}
	// Only return to pool if capacity is reasonable (avoid pooling huge buffers)
	if cap(b.buf) <= 16*1024*1024 { // 16MB max
		b.reset()
		bufferPoolPtr.Put(b)
	}
}

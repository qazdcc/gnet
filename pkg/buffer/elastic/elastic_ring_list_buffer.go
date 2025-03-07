// Copyright (c) 2021 Andy Pan
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package elastic

import (
	"io"
	"math"

	"github.com/panjf2000/gnet/v2/pkg/buffer/linkedlist"
	"github.com/panjf2000/gnet/v2/pkg/buffer/ring"
	gerrors "github.com/panjf2000/gnet/v2/pkg/errors"
	rbPool "github.com/panjf2000/gnet/v2/pkg/pool/ringbuffer"
)

// Buffer combines ring-buffer and list-buffer.
// Ring-buffer is the top-priority buffer to store response data, gnet will only switch to
// list-buffer if the data size of ring-buffer reaches the maximum(MaxStackingBytes), list-buffer is more
// flexible and scalable, which helps the application reduce memory footprint.
type Buffer struct {
	maxStaticBytes int
	ringBuffer     *ring.Buffer
	listBuffer     linkedlist.Buffer
}

// New instantiates an elastic.Buffer and returns it.
func New(maxStaticBytes int) (*Buffer, error) {
	if maxStaticBytes <= 0 {
		return nil, gerrors.ErrNegativeSize
	}
	return &Buffer{maxStaticBytes: maxStaticBytes, ringBuffer: rbPool.Get()}, nil
}

// Read reads data from the Buffer.
func (mb *Buffer) Read(p []byte) (n int, err error) {
	if mb.ringBuffer == nil {
		return mb.listBuffer.Read(p)
	}
	n, err = mb.ringBuffer.Read(p)
	if mb.ringBuffer.IsEmpty() {
		rbPool.Put(mb.ringBuffer)
		mb.ringBuffer = nil
	}
	if n == len(p) {
		return n, err
	}
	var m int
	m, err = mb.listBuffer.Read(p[n:])
	n += m
	return
}

// Peek returns n bytes as [][]byte, these bytes won't be discarded until Buffer.Discard() is called.
func (mb *Buffer) Peek(n int) [][]byte {
	if mb.ringBuffer == nil {
		return mb.listBuffer.PeekBytesList(n)
	}

	if n <= 0 {
		n = math.MaxInt32
	}
	head, tail := mb.ringBuffer.Peek(n)
	if mb.ringBuffer.Buffered() >= n {
		return [][]byte{head, tail}
	}
	return mb.listBuffer.PeekBytesListWithBytes(n, head, tail)
}

// Discard discards n bytes in this buffer.
func (mb *Buffer) Discard(n int) (discarded int, err error) {
	if mb.ringBuffer == nil {
		return mb.listBuffer.Discard(n)
	}

	rbLen := mb.ringBuffer.Buffered()
	discarded, err = mb.ringBuffer.Discard(n)
	if n <= rbLen {
		if n == rbLen {
			rbPool.Put(mb.ringBuffer)
			mb.ringBuffer = nil
		}
		return
	}
	rbPool.Put(mb.ringBuffer)
	mb.ringBuffer = nil
	n -= rbLen
	var m int
	m, err = mb.listBuffer.Discard(n)
	discarded += m
	return
}

// Write appends data to this buffer.
func (mb *Buffer) Write(p []byte) (n int, err error) {
	if mb.ringBuffer == nil && mb.listBuffer.IsEmpty() {
		mb.ringBuffer = rbPool.Get()
	}

	if !mb.listBuffer.IsEmpty() || mb.ringBuffer.Buffered() >= mb.maxStaticBytes {
		mb.listBuffer.PushBytesBack(p)
		return len(p), nil
	}
	if mb.ringBuffer.Len() >= mb.maxStaticBytes {
		writable := mb.ringBuffer.Available()
		if n = len(p); n > writable {
			_, _ = mb.ringBuffer.Write(p[:writable])
			mb.listBuffer.PushBytesBack(p[writable:])
			return
		}
	}
	return mb.ringBuffer.Write(p)
}

// Writev appends multiple byte slices to this buffer.
func (mb *Buffer) Writev(bs [][]byte) (int, error) {
	if mb.ringBuffer == nil && mb.listBuffer.IsEmpty() {
		mb.ringBuffer = rbPool.Get()
	}

	if !mb.listBuffer.IsEmpty() || mb.ringBuffer.Buffered() >= mb.maxStaticBytes {
		var n int
		for _, b := range bs {
			mb.listBuffer.PushBytesBack(b)
			n += len(b)
		}
		return n, nil
	}

	writable := mb.ringBuffer.Available()
	if mb.ringBuffer.Len() < mb.maxStaticBytes {
		writable = mb.maxStaticBytes - mb.ringBuffer.Buffered()
	}
	var pos, cum int
	for i, b := range bs {
		pos = i
		cum += len(b)
		if len(b) > writable {
			_, _ = mb.ringBuffer.Write(b[:writable])
			mb.listBuffer.PushBytesBack(b[writable:])
			break
		}
		n, _ := mb.ringBuffer.Write(b)
		writable -= n
	}
	for pos++; pos < len(bs); pos++ {
		cum += len(bs[pos])
		mb.listBuffer.PushBytesBack(bs[pos])
	}
	return cum, nil
}

// ReadFrom implements io.ReaderFrom.
func (mb *Buffer) ReadFrom(r io.Reader) (int64, error) {
	if mb.ringBuffer == nil && mb.listBuffer.IsEmpty() {
		mb.ringBuffer = rbPool.Get()
	}
	if !mb.listBuffer.IsEmpty() || mb.ringBuffer.Buffered() >= mb.maxStaticBytes {
		return mb.listBuffer.ReadFrom(r)
	}
	return mb.ringBuffer.ReadFrom(r)
}

// WriteTo implements io.WriterTo.
func (mb *Buffer) WriteTo(w io.Writer) (n int64, err error) {
	if mb.ringBuffer == nil {
		return mb.listBuffer.WriteTo(w)
	}
	n, err = mb.ringBuffer.WriteTo(w)
	if mb.ringBuffer.IsEmpty() {
		rbPool.Put(mb.ringBuffer)
		mb.ringBuffer = nil
	}
	if err != nil {
		return
	}
	var m int64
	m, err = mb.listBuffer.WriteTo(w)
	n += m
	return
}

// Buffered returns the number of bytes that can be read from the current buffer.
func (mb *Buffer) Buffered() int {
	if mb.ringBuffer == nil {
		return mb.listBuffer.Buffered()
	}
	return mb.ringBuffer.Buffered() + mb.listBuffer.Buffered()
}

// IsEmpty indicates whether this buffer is empty.
func (mb *Buffer) IsEmpty() bool {
	if mb.ringBuffer == nil {
		return mb.listBuffer.IsEmpty()
	}

	return mb.ringBuffer.IsEmpty() && mb.listBuffer.IsEmpty()
}

// Reset resets the buffer.
func (mb *Buffer) Reset(maxStaticBytes int) {
	if mb.ringBuffer != nil {
		mb.ringBuffer.Reset()
	}
	mb.listBuffer.Reset()

	if maxStaticBytes > 0 {
		mb.maxStaticBytes = maxStaticBytes
	}
}

// Release frees all resource of this buffer.
func (mb *Buffer) Release() {
	if mb.ringBuffer != nil {
		rbPool.Put(mb.ringBuffer)
		mb.ringBuffer = nil
	}

	mb.listBuffer.Reset()
}

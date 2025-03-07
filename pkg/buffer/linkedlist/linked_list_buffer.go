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

package linkedlist

import (
	"io"
	"math"

	bbPool "github.com/panjf2000/gnet/v2/pkg/pool/bytebuffer"
)

// ByteBuffer is the node of the linked list of bytes.
type ByteBuffer struct {
	Buf  *bbPool.ByteBuffer
	next *ByteBuffer
}

// Len returns the length of ByteBuffer.
func (b *ByteBuffer) Len() int {
	if b.Buf == nil {
		return -1
	}
	return b.Buf.Len()
}

// IsEmpty indicates whether the ByteBuffer is empty.
func (b *ByteBuffer) IsEmpty() bool {
	if b.Buf == nil {
		return true
	}
	return b.Buf.Len() == 0
}

// Buffer is a linked list of ByteBuffer.
type Buffer struct {
	bs    [][]byte
	head  *ByteBuffer
	tail  *ByteBuffer
	size  int
	bytes int
}

// Read reads data from the Buffer.
func (llb *Buffer) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	for b := llb.Pop(); b != nil; b = llb.Pop() {
		m := copy(p[n:], b.Buf.B)
		n += m
		if m < b.Len() {
			b.Buf.B = b.Buf.B[m:]
			llb.PushFront(b)
		} else {
			bbPool.Put(b.Buf)
		}
		if n == len(p) {
			return
		}
	}
	return
}

// Pop returns and removes the head of l. If l is empty, it returns nil.
func (llb *Buffer) Pop() *ByteBuffer {
	if llb.head == nil {
		return nil
	}
	b := llb.head
	llb.head = b.next
	if llb.head == nil {
		llb.tail = nil
	}
	b.next = nil
	llb.size--
	llb.bytes -= b.Buf.Len()
	return b
}

// PushFront adds the new node to the head of l.
func (llb *Buffer) PushFront(b *ByteBuffer) {
	if b == nil {
		return
	}
	if llb.head == nil {
		b.next = nil
		llb.tail = b
	} else {
		b.next = llb.head
	}
	llb.head = b
	llb.size++
	llb.bytes += b.Buf.Len()
}

// PushBack adds a new node to the tail of l.
func (llb *Buffer) PushBack(b *ByteBuffer) {
	if b == nil {
		return
	}
	if llb.tail == nil {
		llb.head = b
	} else {
		llb.tail.next = b
	}
	b.next = nil
	llb.tail = b
	llb.size++
	llb.bytes += b.Buf.Len()
}

// PushBytesFront is a wrapper of PushFront, which accepts []byte as its argument.
func (llb *Buffer) PushBytesFront(p []byte) {
	if len(p) == 0 {
		return
	}
	bb := bbPool.Get()
	_, _ = bb.Write(p)
	llb.PushFront(&ByteBuffer{Buf: bb})
}

// PushBytesBack is a wrapper of PushBack, which accepts []byte as its argument.
func (llb *Buffer) PushBytesBack(p []byte) {
	if len(p) == 0 {
		return
	}
	bb := bbPool.Get()
	_, _ = bb.Write(p)
	llb.PushBack(&ByteBuffer{Buf: bb})
}

// PeekBytesList assembles the up to maxBytes of [][]byte based on the list of ByteBuffer,
// it won't remove these nodes from l until Discard() is called.
func (llb *Buffer) PeekBytesList(maxBytes int) [][]byte {
	if maxBytes <= 0 {
		maxBytes = math.MaxInt32
	}
	llb.bs = llb.bs[:0]
	var cum int
	for iter := llb.head; iter != nil; iter = iter.next {
		llb.bs = append(llb.bs, iter.Buf.B)
		if cum += iter.Buf.Len(); cum >= maxBytes {
			break
		}
	}
	return llb.bs
}

// PeekBytesListWithBytes is like PeekBytesList but accepts [][]byte and puts them onto head.
func (llb *Buffer) PeekBytesListWithBytes(maxBytes int, bs ...[]byte) [][]byte {
	if maxBytes <= 0 {
		maxBytes = math.MaxInt32
	}
	llb.bs = llb.bs[:0]
	var cum int
	for _, b := range bs {
		if n := len(b); n > 0 {
			llb.bs = append(llb.bs, b)
			if cum += n; cum >= maxBytes {
				return llb.bs
			}
		}
	}
	for iter := llb.head; iter != nil; iter = iter.next {
		llb.bs = append(llb.bs, iter.Buf.B)
		if cum += iter.Buf.Len(); cum >= maxBytes {
			break
		}
	}
	return llb.bs
}

// Discard removes some nodes based on n bytes.
func (llb *Buffer) Discard(n int) (discarded int, err error) {
	if n <= 0 {
		return
	}
	for n != 0 {
		b := llb.Pop()
		if b == nil {
			break
		}
		if n < b.Len() {
			b.Buf.B = b.Buf.B[n:]
			discarded += n
			llb.PushFront(b)
			break
		}
		n -= b.Len()
		discarded += b.Len()
		bbPool.Put(b.Buf)
	}
	return
}

const minRead = 512

// ReadFrom implements io.ReaderFrom.
func (llb *Buffer) ReadFrom(r io.Reader) (n int64, err error) {
	var m int
	for {
		bb := bbPool.Get()
		bb.B = bb.B[:cap(bb.B)]
		if len(bb.B) == 0 {
			bb.B = make([]byte, minRead)
		}
		m, err = r.Read(bb.B)
		if m < 0 {
			panic("Buffer.ReadFrom: reader returned negative count from Read")
		}
		n += int64(m)
		bb.B = bb.B[:m]
		if err == io.EOF {
			bbPool.Put(bb)
			return n, nil
		}
		if err != nil {
			bbPool.Put(bb)
			return
		}
		llb.PushBack(&ByteBuffer{Buf: bb})
	}
}

// WriteTo implements io.WriterTo.
func (llb *Buffer) WriteTo(w io.Writer) (n int64, err error) {
	var m int
	for b := llb.Pop(); b != nil; b = llb.Pop() {
		m, err = w.Write(b.Buf.B)
		if m > b.Len() {
			panic("Buffer.WriteTo: invalid Write count")
		}
		n += int64(m)
		if err != nil {
			return
		}
		if m < b.Len() {
			b.Buf.B = b.Buf.B[m:]
			llb.PushFront(b)
			return n, io.ErrShortWrite
		}
	}
	return
}

// Len returns the length of the list.
func (llb *Buffer) Len() int {
	return llb.size
}

// Buffered returns the number of bytes that can be read from the current buffer.
func (llb *Buffer) Buffered() int {
	return llb.bytes
}

// IsEmpty reports whether l is empty.
func (llb *Buffer) IsEmpty() bool {
	return llb.head == nil
}

// Reset removes all elements from this list.
func (llb *Buffer) Reset() {
	for b := llb.Pop(); b != nil; b = llb.Pop() {
		bbPool.Put(b.Buf)
	}
	llb.head = nil
	llb.tail = nil
	llb.size = 0
	llb.bytes = 0
	llb.bs = llb.bs[:0]
}

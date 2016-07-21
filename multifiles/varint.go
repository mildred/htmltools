package multifiles

import (
	"encoding/binary"
	"io"
)

type byteReader struct {
	io.Reader
}

func (r *byteReader) ReadByte() (c byte, err error) {
	buf := make([]byte, 1)
	_, err = io.ReadFull(r, buf)
	c = buf[0]
	return
}

func readUvarint(r io.Reader) (uint64, error) {
	return binary.ReadUvarint(&byteReader{r})
}

func readVarint(r io.Reader) (int64, error) {
	return binary.ReadVarint(&byteReader{r})
}

func writeUvarint(w io.Writer, x uint64) (int, error) {
	buf := make([]byte, 10)
	i := binary.PutUvarint(buf, x)
	return w.Write(buf[:i])
}

func writeVarint(w io.Writer, x int64) (int, error) {
	buf := make([]byte, 10)
	i := binary.PutVarint(buf, x)
	return w.Write(buf[:i])
}

func appendUvarint(buf []byte, x uint64) []byte {
	b := make([]byte, 10)
	i := binary.PutUvarint(b, x)
	return append(buf, b[:i]...)
}

func appendVarint(buf []byte, x int64) []byte {
	b := make([]byte, 10)
	i := binary.PutVarint(b, x)
	return append(buf, b[:i]...)
}

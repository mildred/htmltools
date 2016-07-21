package multifiles

import (
	"errors"
	"io"
)

// ReadHeader reads a multicodec header from a reader.
// Returns the header found, or an error if the header
// mismatched.
func readHeader(r io.Reader) (path []byte, err error) {
	lbuf := make([]byte, 1)
	if _, err := r.Read(lbuf); err != nil {
		return lbuf, err
	}

	l := int(lbuf[0])
	if l > 127 {
		return lbuf, ErrVarints
	}

	buf := make([]byte, l+1)
	buf[0] = lbuf[0]
	if _, err := io.ReadFull(r, buf[1:]); err != nil {
		return buf, err
	}
	if buf[l] != '\n' {
		return buf, ErrHeaderInvalid
	}
	return buf, nil
}

// HeaderPath returns the multicodec path from header
func headerPath(hdr []byte) string {
	hdr = hdr[1:]
	if hdr[len(hdr)-1] == '\n' {
		hdr = hdr[:len(hdr)-1]
	}
	return string(hdr)
}

// Header returns a multicodec header with the given path.
func header(path string) []byte {
	p := []byte(path)
	l := len(p) + 1 // + \n
	if l >= 127 {
		panic(ErrVarints.Error())
	}

	buf := make([]byte, l+1)
	buf[0] = byte(l)
	copy(buf[1:], p)
	buf[l] = '\n'
	return buf
}

var (
	ErrHeaderInvalid = errors.New("multicodec header invalid")
	ErrVarints       = errors.New("multicodec varints not yet implemented")
)

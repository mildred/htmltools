package multifiles

import (
	"bytes"
	"io"
)

type multiReader struct {
	readers []io.Reader
}

func (mr *multiReader) Read(p []byte) (n int, err error) {
	for len(mr.readers) > 0 {
		n, err = mr.readers[0].Read(p)
		if n > 0 || err != io.EOF {
			if err == io.EOF {
				// Don't return EOF yet. There may be more bytes
				// in the remaining readers.
				err = nil
			}
			return
		}
		mr.readers = mr.readers[1:]
	}
	return 0, io.EOF
}

func (mr *multiReader) Unread(r io.Reader) {
	mr.readers = append([]io.Reader{r}, mr.readers...)
}

func (mr *multiReader) UnreadBytes(b []byte) {
	mr.Unread(bytes.NewReader(b))
}

// MultiReader returns a Reader that's the logical concatenation of
// the provided input readers.  They're read sequentially.  Once all
// inputs have returned EOF, Read will return EOF.  If any of the readers
// return a non-nil, non-EOF error, Read will return that error.
func newMultiReader(readers ...io.Reader) *multiReader {
	r := make([]io.Reader, len(readers))
	copy(r, readers)
	return &multiReader{r}
}

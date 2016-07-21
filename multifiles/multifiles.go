package multifiles

import (
	"errors"
	"io"
	"path/filepath"
)

func NewReader(r io.Reader, name string) *Reader {
	return &Reader{newMultiReader(r), true, ModeEOF, 0, name, ""}
}

type Mode uint64

const (
	ModeEOF  Mode = 0
	ModeFlat Mode = 1
	ModeSize Mode = 2
)

type Reader struct {
	r     *multiReader
	start bool
	mode  Mode
	avail uint64
	name  string
	sname string
}

func (r *Reader) Next() error {
	if r.start {
		p, err := readHeader(r.r)
		if err != nil && err != ErrVarints && err != ErrHeaderInvalid {
			return err
		} else if err != nil || headerPath(p) != "/multifile" {
			r.r.UnreadBytes(p)
			r.mode = ModeFlat
			r.start = false
			r.sname = r.name
			return nil
		}
		r.start = false
	} else if r.mode != ModeEOF {
		buf := make([]byte, 100000)
		for {
			_, err := r.Read(buf)
			if err == io.EOF {
				break
			} else if err != nil {
				return err
			}
		}
	}
	if r.mode == ModeFlat {
		return io.EOF
	}
	name, err := r.ReadSizedChunk()
	if err != nil {
		return err
	}
	r.mode = ModeSize
	r.sname = string(name)
	return nil
}

func (r *Reader) Name() string {
	return r.sname
}

func (r *Reader) FileName() string {
	return filepath.Join(filepath.Dir(r.name), r.sname)
}

func (r *Reader) Mode() Mode {
	return r.mode
}

func (r *Reader) Read(p []byte) (n int, err error) {
begin:
	switch r.mode {
	default:
	case ModeEOF:
		return 0, io.EOF

	case ModeFlat:
		return r.r.Read(p)

	case ModeSize:
		if r.avail == 0 {
			n, err := readUvarint(r.r)
			if err != nil {
				return 0, err
			}
			r.avail = n
			if n == 0 {
				err := r.readMode()
				if err != nil {
					return 0, err
				}
				goto begin
			}
		}

		if r.avail < uint64(len(p)) {
			p = p[:r.avail]
		}
		n, err := r.Read(p)
		r.avail = r.avail - uint64(n)
		return n, err
	}
	return 0, nil
}

func (r *Reader) readMode() error {
	mode, err := readUvarint(r.r)
	if err != nil {
		r.mode = Mode(mode)
	}
	return err
}

func (r *Reader) ReadSizedChunk() ([]byte, error) {
	n, err := readUvarint(r.r)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, n)
	_, err = io.ReadFull(r.r, buf)
	return buf, err
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w, true, "", false}
}

type Writer struct {
	w     io.Writer
	start bool
	name  string
	flat  bool
}

var (
	ErrSetFlatNonEmptyFile = errors.New("Cannot set flat mode after the first byte")
	ErrNextOnFlatMode      = errors.New("Cannot go to next file in flat mode")
)

func (w *Writer) SetFlat() {
	if !w.start {
		panic(ErrSetFlatNonEmptyFile)
	}

	w.flat = true
	w.start = false
}

func (w *Writer) Write(p []byte) (int, error) {
	if w.flat {
		return w.w.Write(p)
	} else if w.start {
		_, err := w.Write(header("/multifile"))
		if err != nil {
			return 0, err
		}
		w.start = false
		err = w.WriteSizedChunk([]byte(w.name))
		if err != nil {
			return 0, err
		}
	}
	if len(p) == 0 {
		return 0, nil
	}
	_, err := writeUvarint(w.w, uint64(len(p)))
	if err != nil {
		return 0, err
	}
	return w.Write(p)
}

func (w *Writer) Next(name string) error {
	if w.flat {
		return ErrNextOnFlatMode
	} else if w.start {
		w.name = name
		return nil
	}

	buf := appendUvarint(appendUvarint(nil, 0), uint64(ModeEOF))
	buf = append(buf, []byte(name)...)
	_, err := w.w.Write(buf)
	if err != nil {
		return err
	}
	return nil
}

func (w *Writer) WriteSizedChunk(buf []byte) error {
	_, err := writeUvarint(w.w, uint64(len(buf)))
	if err != nil {
		return err
	}
	_, err = w.w.Write(buf)
	return err
}

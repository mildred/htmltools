package main

import (
	"flag"
	"fmt"
	"github.com/golang-commonmark/markdown"
	"github.com/mildred/htmltools/parser"
	"golang.org/x/net/html"
	"io"
	"os"
	"path/filepath"
)

func main() {
	flag.Parse()
	infile := flag.Arg(0)

	if infile == "-" {
		infile = ""
	}

	var f1 io.Reader
	var dir string
	if infile == "" {
		f1 = os.Stdin
		dir = "."
	} else {
		f, err := os.Open(infile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
			return
		}
		defer f.Close()
		f1 = f
		dir = filepath.Dir(infile)
	}

	err := handleTags(dir, f1, os.Stdout)
	if err != nil && err != io.EOF {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	os.Exit(0)
}

func handleTags(curdir string, f1 io.Reader, f2 io.Writer) error {
	p := parser.NewParser(f1)
	for {

		err := p.Next()
		if err != nil {
			return err
		}

		var raw []byte = p.Raw()

		if p.Type() == html.StartTagToken && p.Data() == "markdown" {

			data, err := p.RawContent()
			if err != nil {
				return err
			}

			md := markdown.New(markdown.XHTMLOutput(true))
			raw = []byte(md.RenderToString(data))

		}

		if raw != nil {
			_, err = f2.Write(raw)
			if err != nil {
				return err
			}
		}

	}
}

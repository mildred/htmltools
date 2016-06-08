package main

import (
	"flag"
	"fmt"
	"io"
	"launchpad.net/xmlpath"
	"os"
	"path/filepath"
)

func main() {
	html := flag.Bool("html", false, "HTML Mode")
	delim := flag.String("d", "", "Record delimiter")
	flag.Parse()
	path := flag.Arg(0)
	infile := flag.Arg(1)

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

	err := handleTags(dir, f1, os.Stdout, *html, path, *delim)
	if err != nil && err != io.EOF {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	os.Exit(0)
}

func handleTags(curdir string, r io.Reader, w io.Writer, html bool, pathstr, delim string) error {
	var n *xmlpath.Node
	var err error
	if html {
		n, err = xmlpath.ParseHTML(r)
	} else {
		n, err = xmlpath.Parse(r)
	}
	if err != nil {
		return err
	}

	path, err := xmlpath.Compile(pathstr)
	if err != nil {
		return err
	}

	i := path.Iter(n)
	for i.Next() {
		fmt.Printf("%s%s", i.Node().XML(), delim)
	}

	return nil
}

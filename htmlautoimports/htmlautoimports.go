package main

import (
	"bytes"
	"flag"
	"fmt"
	"golang.org/x/net/html"
	"io/ioutil"
	"os"
	"path/filepath"
)

func readImports(imports []string) (res map[string][]byte, err error) {
	res = map[string][]byte{}
	for _, fname := range imports {
		res[fname], err = ioutil.ReadFile(fname)
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

func replaceImports(fname string, imports map[string][]byte) error {
	data, err := ioutil.ReadFile(fname)
	if err != nil {
		return err
	}

	for ifile, idata := range imports {
		source, err := filepath.Rel(filepath.Dir(fname), ifile)
		if err != nil {
			return err
		}
		import_tag := fmt.Sprintf("<include-file src=\"%s\" />", html.EscapeString(source))
		data = bytes.Replace(data, idata, []byte(import_tag), -1)
	}

	return ioutil.WriteFile(fname, data, os.ModePerm)
}

func main() {
	flag.Parse()

	imports := flag.Args()
	infile := imports[len(imports)-1]
	imports = imports[:len(imports)-1]

	importmap, err := readImports(imports)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error)
		os.Exit(1)
	}

	err = replaceImports(infile, importmap)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error)
		os.Exit(1)
	}

	os.Exit(0)
}

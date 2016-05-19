package main

import (
	"flag"
	"fmt"
	"golang.org/x/net/html"
	"io"
	"os"
	"path/filepath"
)

func main() {
	flag.Parse()
	infile := flag.Arg(0)
	outfile := flag.Arg(1)

	err := handleTags(infile, outfile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	os.Exit(0)
}

func readAttributeSrc(z *html.Tokenizer, attrs bool) (src string) {
	for attrs {
		var key, val []byte
		key, val, attrs = z.TagAttr()
		if string(key) == "src" {
			src = string(val)
		}
	}
	return
}

func handleTags(infile, outfile string) error {
	err := os.MkdirAll(filepath.Dir(outfile), os.ModePerm)
	if err != nil {
		return err
	}

	f1, err := os.Open(infile)
	if err != nil {
		return err
	}
	defer f1.Close()

	f2, err := os.Create(outfile)
	if err != nil {
		return err
	}
	defer f2.Close()

	z := html.NewTokenizer(f1)

	for {
		tk := z.Next()
		if tk == html.ErrorToken {
			err := z.Err()
			if err != io.EOF {
				return err
			}
			break
		}

		raw0 := z.Raw()
		rawData := make([]byte, len(raw0))
		copy(rawData, raw0)

		if tk == html.SelfClosingTagToken {
			tagName, attrs := z.TagName()

			if string(tagName) == "include-file" {
				src := readAttributeSrc(z, attrs)
				if src != "" {
					target := filepath.Join(filepath.Dir(infile), src)
					f, err := os.Open(target)
					if err != nil {
						return err
					}
					defer f.Close()
					_, err = io.Copy(f2, f)
					if err != nil {
						return err
					}
					continue
				}
			}
		}

		_, err = f2.Write(rawData)
		if err != nil {
			return err
		}
	}
	return nil
}

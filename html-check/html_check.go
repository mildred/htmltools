package main

import (
	"flag"
	"fmt"
	"golang.org/x/net/html"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	flag.Parse()
	infile := flag.Arg(0)

	if infile == "-" {
		infile = ""
	}

	err := handleTags(".", infile, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	os.Exit(0)
}

func readAttributeXmlBase(z *html.Tokenizer, attrs bool) (src string) {
	for attrs {
		var key, val []byte
		key, val, attrs = z.TagAttr()
		if string(key) == "xml:base" {
			src = string(val)
		}
	}
	return
}

func handleTags(curdir, infile string, f2 io.Writer) error {
	var f1 io.Reader
	if infile == "" {
		f1 = os.Stdin
	} else {
		f, err := os.Open(infile)
		if err != nil {
			return err
		}
		defer f.Close()
		f1 = f
		curdir = filepath.Join(curdir, filepath.Dir(infile))
	}

	breadcrumb := []string{}
	z := html.NewTokenizer(f1)

	errors := 0

	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			err := z.Err()
			if err != io.EOF {
				return err
			}
			break
		}

		raw0 := z.Raw()
		rawData := make([]byte, len(raw0))
		copy(rawData, raw0)

		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			t := z.Token()
			breadcrumb = append(breadcrumb, t.Data)
			rawData = []byte(t.String())
		}

		_, err := f2.Write(rawData)
		if err != nil {
			return err
		}

		if tt == html.EndTagToken || tt == html.SelfClosingTagToken {
			t := z.Token()
			if t.Data != "" && t.Data != breadcrumb[len(breadcrumb)-1] {
				fmt.Fprintf(os.Stderr, "%s: Incorrect closing tag: expected %s\n", strings.Join(breadcrumb, "/"), t.Data)
				errors += 1
				for len(breadcrumb) > 0 && breadcrumb[len(breadcrumb)-1] != t.Data {
					breadcrumb = breadcrumb[:len(breadcrumb)-1]
				}
			}
			rawData = []byte(t.String())
			breadcrumb = breadcrumb[:len(breadcrumb)-1]
		}

	}

	if len(breadcrumb) > 0 {
		fmt.Fprintf(os.Stderr, "%s: Unexpected enf of file", strings.Join(breadcrumb, "/"))
		errors += 1
	}

	if errors > 0 {
		if infile != "" {
			return fmt.Errorf("%s: There are %d errors", infile, errors)
		} else {
			return fmt.Errorf("There are %d errors", infile, errors)
		}
	}

	return nil
}

func getURL(tag, attr, val string) *url.URL {
	if attr == "src" || attr == "href" {
		u, err := url.Parse(val)
		if err != nil {
			return nil
		}
		return u
	}
	return nil
}

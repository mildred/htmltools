package main

import (
	"flag"
	"fmt"
	"github.com/mildred/htmltools/parser"
	"golang.org/x/net/html"
	"io"
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
	var links []*html.Token
	p := parser.NewParser(f1)
	for {

		err := p.Next()
		if err != nil {
			return err
		}

		var raw []byte = p.Raw()
		var path string = strings.Join(p.Breadcrumb(), "/")

		//fmt.Fprintf(os.Stderr, "%v: is start %v\n", path, p.IsStartTag())
		if path == "html/head/link" && p.IsStartTag() {
			links = append(links, p.Token())
		}
		if p.Type() == html.StartTagToken && p.Data() == "backlink-list" {

			//fmt.Fprintf(os.Stderr, "template-instance: %v\n", string(raw))
			raw = nil
			rel := p.Attr("rel")
			rev := p.Attr("rev")
			attrs := p.Token().Attr
			//fmt.Fprintf(os.Stderr, "%d attributes: %#v\n", len(attrs), string(p.Raw()))
			//fmt.Fprintf(os.Stderr, "%d attributes: %#v\n", len(attrs), p.Token())

			data, err := p.RawContent()
			if err != nil {
				return err
			}

			//fmt.Fprintf(os.Stderr, "%d links\n", len(links))
			for _, l := range links {
				//fmt.Fprintf(os.Stderr, "link: %v\n", l.String())

				href := parser.Attr(l, "href")
				rel2 := parser.Attr(l, "rel")
				rev2 := parser.Attr(l, "rev")
				if rel != nil && (rel2 == nil || rel.Val != rel2.Val) {
					continue
				}
				if rev != nil && (rev2 == nil || rev.Val != rev2.Val) {
					continue
				}
				if href == nil {
					continue
				}

				raw = append(raw, []byte("<template-instance src=\"")...)
				raw = append(raw, []byte(html.EscapeString(href.Val))...)
				raw = append(raw, '"')

				//fmt.Fprintf(os.Stderr, "%d attributes:\n", len(attrs))
				for _, a := range attrs {
					//fmt.Fprintf(os.Stderr, " - %#v\n", a)
					if a.Key == "rel" || a.Key == "rev" || a.Key == "src" || a.Key == "id" {
						continue
					}
					key := a.Key

					raw = append(raw, ' ')
					raw = append(raw, []byte(key)...)
					raw = append(raw, '=', '"')
					raw = append(raw, []byte(html.EscapeString(a.Val))...)
					raw = append(raw, '"')
				}
				raw = append(raw, '>')

				raw = append(raw, data...)
				raw = append(raw, []byte("</template-instance>")...)
			}

		}

		if raw != nil {
			_, err = f2.Write(raw)
			if err != nil {
				return err
			}
		}

	}
}

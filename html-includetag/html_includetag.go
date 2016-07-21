package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/mildred/htmltools/htmldepth"
	"github.com/mildred/htmltools/multifiles"
	"github.com/mildred/htmltools/parser"
	"golang.org/x/net/html"
	"io"
	"os"
	"path/filepath"
)

func main() {
	chdir := flag.String("C", "", "Change directory before operation")
	flag.Parse()
	infile := flag.Arg(0)

	if infile == "-" {
		infile = ""
	}

	if *chdir != "" {
		err := os.Chdir(*chdir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	}

	var r *multifiles.Reader
	var w *multifiles.Writer = multifiles.NewWriter(os.Stdout)
	if infile == "" {
		r = multifiles.NewReader(os.Stdin, "")
	} else {
		f, err := os.Open(infile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
			return
		}
		defer f.Close()
		r = multifiles.NewReader(f, infile)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	for err = r.Next(); err != io.EOF; err = r.Next() {
		dir := filepath.Dir(r.FileName())
		err = w.Next(r.Name())
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		if r.Mode() == multifiles.ModeFlat {
			w.SetFlat()
		}
		err = os.Chdir(dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		err = handleTags(dir, "", r, w, []Content{Content{"", nil}})
		if err != nil && err != io.EOF {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		err = os.Chdir(cwd)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	}
	if err != nil && err != io.EOF {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	os.Exit(0)
}

type Content struct {
	Base    string
	Content []byte
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

func readTagContent(z *html.Tokenizer, d *htmldepth.HTMLDepth) ([]byte, error) {
	var data []byte
	var depth int = d.Depth()

	for {
		tk := z.Next()
		if tk == html.ErrorToken {
			return nil, z.Err()
		}

		raw0 := z.Raw()
		rawData := make([]byte, len(raw0))
		copy(rawData, raw0)
		t := z.Token()

		if tk == html.StartTagToken || tk == html.SelfClosingTagToken {
			//fmt.Fprintf(os.Stderr, "+ %d %v\n", d.Depth(), string(rawData))
			d.Start(string(t.Data))
		}

		if tk == html.EndTagToken || tk == html.SelfClosingTagToken {
			//fmt.Fprintf(os.Stderr, "- %d %v\n", d.Depth(), string(rawData))
			err := d.Stop(string(t.Data))
			if err != nil {
				return nil, err
			}
			if depth > d.Depth() {
				return data, nil
			}
		}

		data = append(data, rawData...)
	}
}

func handleTags(curdir, xmlBase string, f1 io.Reader, f2 io.Writer, content_stack []Content) error {
	p := parser.NewParser(f1)
	//fmt.Fprintf(os.Stderr, "handle-tags(%#v, %#v)\n", curdir, xmlBase)

	for {
		err := p.Next()
		if err != nil {
			return err
		}

		rawData := p.Raw()
		silent := false
		addXmlBase := true

		if p.IsStartTag() && p.Data() == "include-file" {
			var src string
			var base string
			if p.Depth() == 1 {
				base = xmlBase
			}
			src = p.AttrVal("src", "")
			base = p.AttrVal("xml:base", base)
			//fmt.Fprintf(os.Stderr, "include-file %#v %#v %#v\n", src, base, p.Token().String())

			var inData []byte
			if p.Token().Type == html.StartTagToken {
				var err error
				inData, err = p.RawContent()
				if err != nil {
					return err
				}
			}
			if src != "" {
				abssrcdir, err := filepath.Abs(filepath.Join(curdir, filepath.Dir(src)))
				if err != nil {
					return err
				}
				abscurdir, err := filepath.Abs(curdir)
				if err != nil {
					return err
				}
				revpath, err := filepath.Rel(abssrcdir, abscurdir)
				if err != nil {
					return err
				}
				//fmt.Fprintf(os.Stderr, "fp.Rel(%v %v, %v %v) = %v\n", src, abssrcdir, curdir, abscurdir, revpath)
				f, err := os.Open(filepath.Join(curdir, src))
				if err != nil {
					return err
				}
				defer f.Close()
				//fmt.Fprintf(os.Stderr, "include(%#v) xml:base=Join(%#v, Dir(%#v))=%v\n", src, base, src, filepath.Join(base, filepath.Dir(src)))
				err = handleTags(
					filepath.Join(curdir, filepath.Dir(src)),
					filepath.Join(base, filepath.Dir(src)),
					f, f2,
					append(content_stack, Content{revpath + "/", inData}))
				if err != nil && err != io.EOF {
					return err
				}
				silent = true
				addXmlBase = false
			}
		}
		if p.IsStartTag() && p.IsEndTag() && p.Data() == "include-content" {
			//fmt.Fprintf(os.Stderr, "include-content %#v\n", content_stack)
			var base string
			if p.Depth() == 1 {
				base = xmlBase
			}
			base = p.AttrVal("xml:base", base)
			content := content_stack[len(content_stack)-1]
			err := handleTags(
				filepath.Join(curdir, content.Base),
				filepath.Join(base, content.Base),
				bytes.NewReader(content.Content), f2,
				content_stack[:len(content_stack)-1])
			if err != nil && err != io.EOF {
				return err
			}
			silent = true
			addXmlBase = false
		}

		//fmt.Fprintf(os.Stderr, "%d base(%v) %v %v\n", p.Depth(), xmlBase, addXmlBase, string(rawData))
		if xmlBase != "." && xmlBase != "" && p.Depth() == 1 && addXmlBase {
			//fmt.Fprintf(os.Stderr, "%d base(%v) %v\n", d.Depth(), xmlBase, string(rawData))
			if p.Token().Type == html.StartTagToken {
				//t := z.Token()
				//t.Attr = append(t.Attr, html.Attribute{"", "xml:base", xmlBase})
				//rawData = []byte(t.String())
				rawData = append(rawData[:len(rawData)-1], []byte(" xml:base=\""+html.EscapeString(xmlBase)+"\">")...)
			} else if p.Token().Type == html.SelfClosingTagToken {
				rawData = append(rawData[:len(rawData)-2], []byte(" xml:base=\""+html.EscapeString(xmlBase)+"\" />")...)
			}
			//fmt.Fprintf(os.Stderr, "rawData=%#v\n", string(rawData))
		}

		if !silent {
			_, err := f2.Write(rawData)
			if err != nil {
				return err
			}
		}

	}
}

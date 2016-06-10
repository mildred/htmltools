package main

import (
	"flag"
	"fmt"
	"github.com/mildred/htmltools/parser"
	"golang.org/x/net/html"
	"io"
	"io/ioutil"
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

	err := main2(infile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	os.Exit(0)
}

func main2(infile string) error {
	var r io.ReadSeeker
	var w io.Writer
	var outfile string
	if infile == "" {
		return fmt.Errorf("Expected filename on command line")
	} else {
		fmt.Fprintf(os.Stderr, "Read %s\n", infile)

		f, err := os.Open(infile)
		if err != nil {
			return err
		}
		defer f.Close()
		r = f

		fw, err := ioutil.TempFile(filepath.Dir(infile), filepath.Base(infile))
		if err != nil {
			return err
		}
		defer fw.Close()
		w = fw
		outfile = fw.Name()
	}

	tags, err := getTags(r)
	if err != nil && err != io.EOF {
		return fmt.Errorf("While parsing tags: %v", err)
	}

	fmt.Fprintf(os.Stderr, "detect %d tags\n", len(tags))

	_, err = r.Seek(0, 0)
	if err != nil {
		return err
	}

	p := parser.NewParser(r)
	err = handleTags(p, w, tags)
	if err != nil && err != io.EOF {
		return fmt.Errorf("While adding tags: %v", err)
	}

	if infile != "" {
		err := os.Rename(outfile, infile)
		if err != nil {
			return err
		}
	}

	return nil
}

type Tag struct {
	Name string
	Link string
}

func getTags(r io.Reader) ([]Tag, error) {
	var res []Tag

	p := parser.NewParser(r)
	for {

		err := p.Next()
		if err != nil {
			return res, err
		}

		if p.IsStartTag() && p.Data() == "div" && p.AttrVal("id", "") == "content" {
			res = nil
		}

		if p.IsStartTag() && p.Data() == "a" && p.AttrVal("class", "") == "tag" {
			href := p.AttrVal("href", "")
			data, err := p.RawContent()
			if err != nil {
				return nil, err
			}
			fmt.Fprintf(os.Stderr, "detect %s %s\n", string(data), href)
			res = append(res, Tag{
				Name: string(data),
				Link: href,
			})
		}
	}
}

func handleTags(p *parser.Parser, w io.Writer, tags []Tag) error {
	last_text := ""

	for {

		err := p.Next()
		if err != nil {
			return err
		}

		data := p.Raw()

		if p.Type() == html.TextToken {
			last_text = string(p.Data())
		}

		if p.IsStartTag() && p.Data() == "link" && p.AttrVal("rel", "") == "tag" {
			href := p.AttrVal("href", "")
			for i := range tags {
				if tags[i].Link == href {
					fmt.Fprintf(os.Stderr, "exists: %s %s\n", tags[i].Name, tags[i].Link)
					tags[i].Link = ""
				}
			}
		} else if p.IsEndTag() && p.Data() == "head" {
			indent := detectIndent(last_text, 1)
			for _, tag := range tags {
				if tag.Link == "" {
					continue
				}
				fmt.Fprintf(os.Stderr, "add: %s %s\n", tag.Name, tag.Link)
				tag := fmt.Sprintf("%s<link rel=\"tag\" href=\"%s\" title=\"%s\" />\n%s",
					indent,
					html.EscapeString(tag.Link),
					html.EscapeString(tag.Name),
					indent)
				data = append([]byte(tag), data...)
			}
		}

		_, err = w.Write(data)
		if err != nil {
			return err
		}
	}
}

// FIXME: doesn't work with indent != 1
func detectIndent(text string, indent int) string {
	cr := strings.LastIndex(text, "\n")
	if cr+1 >= len(text) {
		return ""
	}
	if cr >= 0 {
		text = text[cr+1:]
	}
	var block []byte
	for i := 0; i < len(text) && len(block) <= len(text)/indent; i++ {
		switch text[i] {
		case ' ', '\t':
			block = append(block, text[i])
		default:
		}
	}
	return string(block)
}

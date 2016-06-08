package main

import (
	"flag"
	"fmt"
	"github.com/mildred/htmltools/parser"
	//"golang.org/x/net/html"
	"bytes"
	"io"
	"launchpad.net/xmlpath"
	"os"
	"path/filepath"
)

func main() {
	chdir := flag.String("C", "", "Change directory before operation")
	flag.Parse()
	infile := flag.Arg(0)

	if *chdir != "" {
		err := os.Chdir(*chdir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
	}

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

func handleTags(curdir string, r io.Reader, w io.Writer) error {
	var templates map[string][]byte = map[string][]byte{}
	p := parser.NewParser(r)
	for {

		err := p.Next()
		if err != nil {
			return err
		}

		var raw []byte = p.Raw()

		//fmt.Fprintf(os.Stderr, "%v: is start %v\n", path, p.IsStartTag())
		if p.IsStartTag() && p.Data() == "template" {
			id := p.Attr("id")
			if id != nil {
				templates[id.Val], err = p.RawContent()
				if err != nil {
					return err
				}
			}
		}

		if p.IsStartTag() && p.Data() == "template-instance" {

			//fmt.Fprintf(os.Stderr, "template-instance: %v\n", string(raw))
			src := p.Attr("src")
			using := p.Attr("using")

			if src != nil && using != nil {
				if template, ok := templates[using.Val]; ok {
					mapping, err := p.RawContent()
					if err != nil {
						return err
					}
					raw, err = evalTemplate(curdir, src.Val, template, mapping)
					if err != nil {
						return err
					}
				}
			}

		}

		if raw != nil {
			_, err = w.Write(raw)
			if err != nil {
				return err
			}
		}

	}
}

var (
	path_children = xmlpath.MustCompile("./child::node()")
)

func evalTemplate(curdir, src string, template, mapping []byte) ([]byte, error) {
	var err error

	srcfile := src
	if !filepath.IsAbs(srcfile) {
		srcfile = filepath.Join(curdir, srcfile)
	}

	sf, err := os.Open(srcfile)
	if err != nil {
		return nil, err
	}
	defer sf.Close()
	in, err := xmlpath.ParseHTML(sf)
	if err != nil {
		return nil, err
	}

	p := parser.NewParser(bytes.NewReader(mapping))

	tt, err := xmlpath.ParseHTML(bytes.NewReader(template))
	if err != nil {
		return nil, err
	}
	t := tt.Ref

	for {
		err := p.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		if p.IsStartTag() && p.Data() == "map" {
			var frompath, topath *xmlpath.Path
			from := p.Attr("from")
			to := p.Attr("to")
			dataattr := p.Attr("data")
			//format := p.Attr("to")
			//multi := p.Attr("multiple")
			//fetch := p.Attr("fetch")
			onlyif := p.Attr("only-if")

			if to != nil {
				topath, err = xmlpath.Compile(to.Val)
				if err != nil {
					return nil, err
				}
			}

			if onlyif != nil && onlyif.Val == "empty" && topath != nil {
				i := topath.Iter(t.Node)
				empty := true
				for i.Next() {
					if path_children.Iter(i.Node()).Next() {
						fmt.Fprintf(os.Stderr, "only-if=empty: skip because %v is not empty\n", to.Val)
						empty = false
						break
					}
				}
				if !empty {
					continue
				}
				fmt.Fprintf(os.Stderr, "only-if=empty: continue because %v is empty\n", to.Val)
			}

			if from != nil {
				frompath, err = xmlpath.Compile(from.Val)
				if err != nil {
					return nil, err
				}
			}

			var nodes []xmlpath.Node

			if nodes == nil && frompath != nil {
				fmt.Fprintf(os.Stderr, "frompath: %s\n", from.Val)
				i := frompath.Iter(in)
				for i.Next() {
					nodes = append(nodes, *i.Node())
					fmt.Fprintf(os.Stderr, "  - %#v\n", string(i.Node().XML()))
				}
				fmt.Fprintf(os.Stderr, "frompath: %s (%d results)\n", from.Val, len(nodes))
			}

			if nodes == nil && dataattr != nil && dataattr.Val == "relative-url" {
				n := xmlpath.CreateTextNode([]byte(src))
				nodes = append(nodes, n)
			}

			if topath != nil && nodes != nil {
				matches := topath.Iter(t.Node).Nodes()
				fmt.Fprintf(os.Stderr, "%d to matches: %#v\n", len(matches), to.Val)
				for _, m := range matches {
					if m.Node.Kind() == xmlpath.StartNode {
						fmt.Fprintf(os.Stderr, "Set %d children\n", len(nodes))
						m.Node.SetChildren(nodes...)
					} else {
						fmt.Fprintf(os.Stderr, "Convert %d nodes to text\n", len(nodes))
						var data []byte
						for _, n := range nodes {
							data = append(data, n.String()...)
						}
						m.Node.SetBytes(data)
					}
				}
				fmt.Fprintf(os.Stderr, "data: %#v\n", string(t.Node.XML()))
			}
		}
	}

	return t.Node.XML(), nil
}

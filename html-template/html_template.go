package main

import (
	"flag"
	"fmt"
	"github.com/jehiah/go-strftime"
	"github.com/mildred/htmltools/parser"
	"github.com/mildred/htmltools/relurl"
	"time"
	//"golang.org/x/net/html"
	"bytes"
	"io"
	"io/ioutil"
	"launchpad.net/xmlpath"
	"os"
	"path/filepath"
)

var verbose bool = false

func log(format string, args ...interface{}) {
	if verbose {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}

func main() {
	chdir := flag.String("C", "", "Change directory before operation")
	verb := flag.Bool("v", false, "Be verbose")
	flag.Parse()
	infile := flag.Arg(0)

	verbose = *verb

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
	var err error

	// Copy input
	r2, err := ioutil.TempFile("", "html-template.temp.html")
	if err != nil {
		return err
	}
	defer r2.Close()
	defer os.Remove(r2.Name())
	_, err = io.Copy(r2, r)
	if err != nil {
		return err
	}

	// Reopen temp file
	r, err = os.Open(r2.Name())
	if err != nil {
		return err
	}

	if !filepath.IsAbs(curdir) {
		curdir, err = filepath.Abs(curdir)
		if err != nil {
			return err
		}
	}

	var templates map[string][]byte = map[string][]byte{}
	p := parser.NewParser(r)
	for {

		err := p.Next()
		if err != nil {
			return err
		}

		var raw []byte = p.Raw()

		//log("%v: is start %v\n", path, p.IsStartTag())
		if p.IsStartTag() && p.Data() == "template" {
			id := p.Attr("id")
			if id != nil {
				templates[id.Val], err = p.RawContent()
				if err != nil {
					return err
				}
				raw = append(raw, templates[id.Val]...)
				raw = append(raw, p.Raw()...)
			}
		}

		if p.IsStartTag() && p.Data() == "template-instance" {

			//log("template-instance: %v\n", string(raw))
			src := p.AttrVal("src", "")
			using := p.Attr("using")

			if using != nil {
				if template, ok := templates[using.Val]; ok {
					mapping, err := p.RawContent()
					if err != nil {
						return err
					}
					if src == "" {
						r2.Seek(0, 0)
						raw, err = evalTemplate(curdir, src, r2, template, mapping)
					} else {
						raw, err = evalTemplate(curdir, src, nil, template, mapping)
					}
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

// curdir: directory where the template file is
// src:    data source relative to curdir
func evalTemplate(curdir, src string, sf io.Reader, template, mapping []byte) ([]byte, error) {
	var err error
	var in, tt *xmlpath.Node

	srcfile := src
	if !filepath.IsAbs(srcfile) {
		srcfile = filepath.Join(curdir, srcfile)
	}

	p := parser.NewParser(bytes.NewReader(mapping))

	tt, err = xmlpath.ParseHTML(bytes.NewReader(template))
	if err != nil {
		return nil, err
	}
	t := tt.Ref

	if sf == nil {
		sff, err := os.Open(srcfile)
		if err != nil {
			return nil, err
		}
		defer sff.Close()
		sf = sff
	}
	in, err = xmlpath.ParseHTML(sf)
	if err != nil {
		return nil, err
	}

	err = runTemplate(curdir, src, p, in, t)
	if err != nil && err != io.EOF {
		return nil, err
	}

	return t.Node.XML(), nil
}

func runTemplate(curdir, src string, p *parser.Parser, in *xmlpath.Node, t *xmlpath.NodeRef) error {
	depth := p.Depth()

	log("\nTemplating %#v\nfrom:  %#v\nusing: %#v\n\n", string(src), string(in.XML()), string(t.Node.XML()))

	for {
		p.End()
		if p.Depth() < depth {
			log("\nDone Templating %#v\nresult: %#v\n\n", src, string(t.Node.XML()))
			return nil
		}

		err := p.Next()
		if err != nil {
			return err
		}

		if p.IsStartTag() && p.Data() == "map" {
			log("Mapping: %#v\n\n", string(p.Token().String()))
			var namespaces map[string]string = nil
			// FIXME: namespaces
			var frompath, topath *xmlpath.Path
			from := p.Attr("from")
			to := p.Attr("to")
			dataattr := p.Attr("data")
			format := p.Attr("format")
			multi := p.Attr("multiple")
			fetch := p.Attr("fetch")
			onlyif := p.Attr("only-if")

			if to != nil {
				topath, err = xmlpath.CompileNS(to.Val, namespaces)
				if err != nil {
					return err
				}
			}

			if onlyif != nil && onlyif.Val == "empty" && topath != nil {
				i := topath.Iter(t.Node)
				empty := true
				for i.Next() {
					if path_children.Iter(i.Node()).Next() {
						log("  only-if=empty: skip because %v is not empty\n", to.Val)
						empty = false
						break
					}
				}
				if !empty {
					continue
				}
				log("  only-if=empty: continue because %v is empty\n", to.Val)
			}

			if from != nil {
				frompath, err = xmlpath.CompileNS(from.Val, namespaces)
				if err != nil {
					return err
				}
			}

			var nodes []xmlpath.Node

			if nodes == nil && frompath != nil {
				log("  frompath: %s\n", from.Val)
				//log("  frompath: %#v\n", string(in.XML()))
				i := frompath.Iter(in)
				for i.Next() {
					nodes = append(nodes, *i.Node())
					log("  - %#v\n", string(i.Node().XML()))
				}
				log("  frompath: %s (%d results)\n", from.Val, len(nodes))
			}

			if nodes == nil && dataattr != nil && dataattr.Val == "relative-url" {
				n := xmlpath.CreateTextNode([]byte(src))
				nodes = append(nodes, n)
			} else if nodes == nil && dataattr != nil && dataattr.Val == "relative-dir" {
				n := xmlpath.CreateTextNode([]byte(filepath.Dir(src) + "/"))
				nodes = append(nodes, n)
			}

			if format != nil {
				switch format.Val {
				default:
					log("  unknown format %#v, aborting mapping\n", format.Val)
					nodes = nil
					break
				case "link-relative":
					data, err := relurl.UrlJoinString(filepath.Dir(src), string(nodesToText(nodes)), curdir)
					if err != nil {
						return err
					}
					log("  convert to relative link: %#v\n", string(data))
					nodes = []xmlpath.Node{xmlpath.CreateTextNode([]byte(data))}
					break
				case "datetime":
					input := string(nodesToText(nodes))
					if input == "" {
						break
					}
					t, err := time.Parse(time.RFC3339, input)
					if err != nil {
						return err
					}
					format := p.AttrVal("strftime", "%c")
					data := strftime.Format(format, t)
					log("  convert to time (%s): %#v\n", format, string(data))
					nodes = []xmlpath.Node{xmlpath.CreateTextNode([]byte(data))}
					break
				}
			}

			if fetch != nil && fetch.Val == "resource" {
				newsrc := string(nodesToText(nodes))
				log("  fetch %#v\n", newsrc)
				if !filepath.IsAbs(newsrc) {
					newsrc = filepath.Join(filepath.Dir(src), newsrc)
				}
				newsrcfile := newsrc
				if !filepath.IsAbs(newsrcfile) {
					newsrcfile = filepath.Join(curdir, newsrcfile)
				}
				log("  file: %#v\n", newsrcfile)

				sf, err := os.Open(newsrcfile)
				if err != nil {
					return err
				}
				defer sf.Close()
				in, err := xmlpath.ParseHTML(sf)
				if err != nil {
					return err
				}

				err = runTemplate(curdir, newsrc, p, in, t)
				if err != nil {
					return err
				}
				nodes = nil
			}

			if topath != nil && nodes != nil {
				// FIXME: set xml:base
				matches := topath.Iter(t.Node).Nodes()
				log("%d to matches: %#v\n", len(matches), to.Val)
				for _, tnode := range matches {

					if multi != nil && multi.Val == "true" {
						log("Multiple (%d) templating of %#v\n", len(nodes), string(tnode.Node.XML()))
						for i, inode := range nodes {
							n := tnode.Node.Copy().Ref
							//log("Insert %#v\n", string(n.Node.XML()))
							//log(" before %#v\n", string(tnode.Node.XML()))
							err := runTemplate(curdir, src, p, &inode, n)
							if err != nil {
								return err
							}
							tnode.Node.InsertBefore(*n.Node)
							log("Multiple templating result %d: %#v\n", i, string(n.Node.XML()))
						}
						tnode.Node.Remove()

					} else if tnode.Node.Kind() == xmlpath.StartNode {
						log("Set %d children\n", len(nodes))
						for i := range nodes {
							log("--> %#v\n", string(nodes[i].XML()))
						}
						tnode.Node.SetChildren(nodes...)
						log("==> %#v\n", string(tnode.Node.XML()))

					} else {
						log("Convert %d nodes to text\n", len(nodes))
						tnode.Node.SetBytes(nodesToText(nodes))
					}

				}
				log("\nMaping Result: %#v\n", string(t.Node.XML()))
			} else {
				log("\nMapping Aborted\n")
			}
		}
	}
}

func nodesToText(nodes []xmlpath.Node) []byte {
	var data []byte
	for _, inode := range nodes {
		data = append(data, inode.String()...)
	}
	return data
}

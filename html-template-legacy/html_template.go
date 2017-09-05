package main

import (
	"flag"
	"fmt"
	"github.com/jehiah/go-strftime"
	"github.com/mildred/htmltools/parser"
	"github.com/mildred/htmltools/relurl"
	"sort"
	"strings"
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

func logv(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
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
			ifClause := p.AttrVal("if", "")

			var template []byte = nil
			if using != nil {
				template = templates[using.Val]
			}

			mapping, err := p.RawContent()
			if err != nil {
				return err
			}

			if template == nil {
				pp := parser.NewParser(bytes.NewReader(mapping))
				for template == nil {
					err := pp.Next()
					if err != nil {
						return err
					}

					if pp.IsStartTag() && pp.Data() == "template" {
						template, err = pp.RawContent()
						if err != nil {
							return err
						}
					}
				}
			}

			raw = append(raw, mapping...)
			raw = append(raw, p.Raw()...)

			if template != nil {
				if src == "" {
					_, err = r2.Seek(0, 0)
					if err != nil {
						return err
					}
					raw, err = evalTemplate(curdir, src, r2, template, mapping, raw, ifClause)
				} else {
					raw, err = evalTemplate(curdir, src, nil, template, mapping, raw, ifClause)
				}
				if err != nil {
					return err
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
func evalTemplate(curdir, src string, sf io.Reader, template, mapping, raw []byte, ifClause string) ([]byte, error) {
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

	if ifClause != "" {
		ifPath, err := xmlpath.Compile(ifClause)
		if err != nil {
			return nil, err
		}
		log("\nApply if clause %#v\nto: %#v", ifClause, in)
		if !ifPath.Exists(in) {
			return raw, nil
		}
	}

	var sortk SortKeys
	err = runTemplate(curdir, src, p, in, t, &sortk)
	if err != nil && err != io.EOF {
		logv("Error: %#v\n", err)
		return nil, err
	}

	return t.Node.XML(), nil
}

type SortKey struct {
	Asc bool
	Key string
}

type SortKeys struct {
	Keys []SortKey
	Node *xmlpath.NodeRef
}

func (self SortKeys) key(i int) *SortKey {
	if i >= len(self.Keys) {
		return nil
	} else {
		return &self.Keys[i]
	}
}

func (self SortKeys) less(other SortKeys) bool {
	for i := 0; i < len(self.Keys) || i < len(other.Keys); i++ {
		sk := self.key(i)
		ok := other.key(i)
		var asc bool
		var sks, oks string
		if sk != nil && ok != nil {
			asc = sk.Asc
			if ok.Asc != sk.Asc {
				panic("sort order undefined")
			}
			sks = sk.Key
			oks = ok.Key
		} else if sk != nil {
			asc = sk.Asc
			sks = sk.Key
		} else if ok != nil {
			asc = ok.Asc
			oks = ok.Key
		} else {
			continue
		}
		c := strings.Compare(sks, oks)
		if c == 0 {
			continue
		}
		if c < 0 {
			return asc
		} else if c > 0 {
			return !asc
		}
	}
	return false
}

type ByKey []SortKeys

func (s ByKey) Len() int           { return len(s) }
func (s ByKey) Swap(a, b int)      { s[a], s[b] = s[b], s[a] }
func (s ByKey) Less(a, b int) bool { return s[a].less(s[b]) }

func runTemplate(curdir, src string, p *parser.Parser, in *xmlpath.Node, tmpl *xmlpath.NodeRef, sortk *SortKeys) error {
	depth := p.Depth()

	log("\n[%d] Templating in file %#v\nfrom source data: %#v\nusing template: %#v\n\n", depth, string(src), string(in.XML()), string(tmpl.Node.XML()))

	for {
		p.End()
		if p.Depth() < depth {
			log("\n[%d]Done Templating %#v\nresult: %#v\n\n", depth, src, string(tmpl.Node.XML()))
			return nil
		}

		err := p.Next()
		if err != nil {
			//log("Error: %#v\n", err)
			return err
		}

		var namespaces map[string]string = nil
		// FIXME: namespaces

		if p.IsStartTag() && p.Data() == "sort" {
			var s SortKey
			var pathStr string
			asc := p.AttrVal("asc", "")
			desc := p.AttrVal("desc", "")
			format := p.AttrVal("format", "")
			if asc != "" && desc == "" {
				s.Asc = true
				pathStr = asc
			} else if asc == "" && desc != "" {
				s.Asc = false
				pathStr = desc
			} else {
				continue
			}

			path, err := xmlpath.CompileNS(pathStr, namespaces)
			if err != nil {
				return err
			}

			nodes := path.Iter(in).Nodes()

			if format != "" {
				nodes, err = formatNodes(curdir, src, format, nodes, p)
				if err != nil {
					return err
				}
			}

			s.Key = string(nodesToText(nodes))

			if s.Key != "" {
				sortk.Keys = append(sortk.Keys, s)
			}

		} else if p.IsStartTag() && p.Data() == "map" {
			log("[%d] Mapping: %v\n\n", depth, string(p.Token().String()))
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
				i := topath.Iter(tmpl.Node)
				empty := true
				for i.Next() {
					if path_children.Iter(i.Node()).Next() {
						log("[%d]   only-if=empty: skip because %v is not empty\n", depth, to.Val)
						empty = false
						break
					}
				}
				if !empty {
					continue
				}
				log("[%d]   only-if=empty: continue because %v is empty\n", depth, to.Val)
			}

			if from != nil {
				frompath, err = xmlpath.CompileNS(from.Val, namespaces)
				if err != nil {
					return err
				}
			}

			var nodes []*xmlpath.NodeRef

			if nodes == nil && frompath != nil {
				log("[%d]   frompath: %s\n", depth, from.Val)
				//log("  frompath: %#v\n", string(in.XML()))
				i := frompath.Iter(in)
				for i.Next() {
					nodes = append(nodes, i.Node().Ref)
					log("[%d]   - %#v\n", depth, string(i.Node().XML()))
				}
				log("[%d]   frompath: %s (%d results)\n", depth, from.Val, len(nodes))
			}

			if nodes == nil && dataattr != nil && dataattr.Val == "relative-url" {
				n := xmlpath.CreateTextNode([]byte(src))
				nodes = append(nodes, n.Ref)
			} else if nodes == nil && dataattr != nil && dataattr.Val == "relative-dir" {
				n := xmlpath.CreateTextNode([]byte(filepath.Dir(src) + "/"))
				nodes = append(nodes, n.Ref)
			}

			if format != nil {
				nodes, err = formatNodes(curdir, src, format.Val, nodes, p)
				if err != nil {
					return err
				}
			}

			if fetch != nil && fetch.Val == "resource" {
				newsrcs := nodesToSlice(nodes)

				submap, err := p.RawContent()
				if err != nil {
					return err
				}

				var sortedNodes []SortKeys

				for i, newsrc := range newsrcs {
					log("[%d]   fetch %#v\n", depth, newsrc)

					if !filepath.IsAbs(newsrc) {
						newsrc = filepath.Join(filepath.Dir(src), newsrc)
					}
					newsrcfile := newsrc
					if !filepath.IsAbs(newsrcfile) {
						newsrcfile = filepath.Join(curdir, newsrcfile)
					}
					log("[%d]   file: %#v\n", depth, newsrcfile)

					n := tmpl.Node.Copy().Ref
					var sort2 SortKeys
					err = func() error {
						sf, err := os.Open(newsrcfile)
						if err != nil {
							return err
						}
						defer sf.Close()
						in, err := xmlpath.ParseHTML(sf)
						if err != nil {
							return fmt.Errorf("%s: %v", newsrcfile, err)
						}

						pp := parser.NewParser(bytes.NewReader(submap))
						err = runTemplate(curdir, newsrc, pp, in, n, &sort2)
						if err != nil && err != io.EOF {
							log("[%d] Fetch resource error: %v\n", depth, err)
							return err
						}
						return nil
					}()
					if err != nil {
						return err
					}
					sort2.Node = n
					sortedNodes = append(sortedNodes, sort2)
					log("[%d] Resource %d/%d %#v templating result: %#v\n", depth, i+1, len(newsrcs), newsrc, string(n.Node.XML()))
				}
				sort.Stable(ByKey(sortedNodes))

				var res []xmlpath.Node
				for _, n := range sortedNodes {
					res = append(res, *n.Node.Node)
				}
				tmpl.Node.ReplaceInner(res...)
				log("[%d] Resource templating result: %#v\n", depth, string(tmpl.Node.XML()))
				nodes = nil
			} else if topath != nil && nodes != nil {
				// FIXME: set xml:base
				matches := topath.Iter(tmpl.Node).Nodes()
				log("[%d] %d to matches: %#v\n", depth, len(matches), to.Val)
				for _, tnode := range matches {

					if multi != nil && multi.Val == "true" {
						log("[%d] Multiple (%d) templating of %#v\n", depth, len(nodes), string(tnode.Node.XML()))
						submap, err := p.RawContent()
						if err != nil {
							return err
						}
						var sortedNodes []SortKeys
						for i, inode := range nodes {
							n := tnode.Node.Copy().Ref
							//log("Insert %#v\n", string(n.Node.XML()))
							//log(" before %#v\n", string(tnode.Node.XML()))
							pp := parser.NewParser(bytes.NewReader(submap))
							var sort2 SortKeys
							err = runTemplate(curdir, src, pp, inode.Node, n, &sort2)
							if err != nil && err != io.EOF {
								log("[%d] Multiple templating error %v\n", depth, err)
								return err
							}
							sort2.Node = n
							sortedNodes = append(sortedNodes, sort2)
							log("[%d] Multiple templating result %d: %#v\n", depth, i, string(n.Node.XML()))
						}
						sort.Stable(ByKey(sortedNodes))
						for _, n := range sortedNodes {
							tnode.Node.InsertBefore(*n.Node.Node)
						}
						tnode.Node.Remove()

					} else if tnode.Node.Kind() == xmlpath.StartNode {
						log("[%d] Set %d children\n", depth, len(nodes))
						var children []xmlpath.Node
						for i := range nodes {
							log("[%d] --> %#v\n", depth, nodes[i])
							log("[%d] --> %#v\n", depth, string(nodes[i].Node.XML()))
							children = append(children, *nodes[i].Node)
						}
						tnode.Node.SetChildren(children...)
						log("[%d] ==> %#v\n", depth, string(tnode.Node.XML()))

					} else {
						log("[%d] Convert %d nodes to text\n", depth, len(nodes))
						tnode.Node.SetBytes(nodesToText(nodes))
					}

				}
				log("\n[%d] Maping Result: %#v\n", depth, string(tmpl.Node.XML()))
			} else {
				log("\n[%d] Mapping Aborted\n", depth)
			}
		}
	}
}

type AttrsInterface interface {
	AttrVal(name, defVal string) string
}

func formatNodes(curdir, src, format string, nodes []*xmlpath.NodeRef, attrs AttrsInterface) ([]*xmlpath.NodeRef, error) {
	log("format nodes before %s %#v\n", format, nodes)
	switch format {
	default:
		log("   unknown format %#v, aborting mapping\n", format)
		nodes = nil
		break
	case "text":
		nodes = []*xmlpath.NodeRef{xmlpath.CreateTextNode(nodesToText(nodes)).Ref}
		break
	case "split":
		text := nodesToText(nodes)
		nodes = nil
		for _, txt := range bytes.Split(text, []byte(" ")) {
			nodes = append(nodes, xmlpath.CreateTextNode(txt).Ref)
		}
		break
	case "link-relative":
		data, err := relurl.UrlJoinString(filepath.Dir(src), string(nodesToText(nodes)), curdir)
		if err != nil {
			return nil, err
		}
		log("   convert to relative link: %#v\n", string(data))
		nodes = []*xmlpath.NodeRef{xmlpath.CreateTextNode([]byte(data)).Ref}
		break
	case "datetime":
		input := string(nodesToText(nodes))
		if input == "" {
			break
		}
		t, err := time.Parse(time.RFC3339, input)
		if err != nil {
			return nil, err
		}
		format := attrs.AttrVal("strftime", "%c")
		data := strftime.Format(format, t)
		log("   convert to time (%s): %#v\n", format, string(data))
		nodes = []*xmlpath.NodeRef{xmlpath.CreateTextNode([]byte(data)).Ref}
		break
	}
	log("format nodes after %#v\n", nodes)
	return nodes, nil
}

func nodesToText(nodes []*xmlpath.NodeRef) []byte {
	var data []byte
	for _, inode := range nodes {
		//log("text for %s: %s\n", string(inode.Node.XML()), string(inode.Node.String()))
		data = append(data, inode.Node.String()...)
	}
	return data
}

func nodesToSlice(nodes []*xmlpath.NodeRef) []string {
	var data []string
	for _, inode := range nodes {
		data = append(data, inode.Node.String())
	}
	return data
}

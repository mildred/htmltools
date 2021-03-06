package main

import (
	"flag"
	"fmt"
	"github.com/jehiah/go-strftime"
	"github.com/mildred/htmltools/parser"
	"github.com/mildred/htmltools/relurl"
	"github.com/mildred/xml-dom"
	"github.com/mildred/xml-dom/xpath"
	"sort"
	"strings"
	"time"
	//"golang.org/x/net/html"
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

var verbose bool = false
var indent = ""

func logIndent() {
	indent = indent + "  "
}

func logDeIndent() {
	if len(indent) >= 2 {
		indent = indent[0 : len(indent)-2]
	}
}

func log(format string, args ...interface{}) {
	if verbose {
		s := fmt.Sprintf(format, args...)
		s = strings.TrimRight(s, "\n")
		s = indent + strings.Replace(s, "\n", "\n"+indent, -1) + "\n"
		fmt.Fprint(os.Stderr, s)
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
					srcfile := src
					if !filepath.IsAbs(srcfile) {
						srcfile = filepath.Join(curdir, srcfile)
					}

					sf, err := os.Open(srcfile)
					if err != nil {
						return err
					}
					defer sf.Close()

					raw, err = evalTemplate(curdir, src, sf, template, mapping, raw, ifClause)
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
	path_children = xpath.MustCompile("./child::node()")
)

// curdir:   directory where the template file is
// src:      data source relative to curdir (empty denotes the file being
//           templated)
// sf:       data source reader
// template: the content of the <template/> tag
// mapping:  the content of the <template-instance/> tag
// raw:      ...
// ifClause: ...
func evalTemplate(curdir, src string, sf io.Reader, template, mapping, raw []byte, ifClause string) ([]byte, error) {
	var err error
	var in, t *xmldom.Node
	// in: XML DOM for sf
	// t:  XML DOM for template

	p := parser.NewParser(bytes.NewReader(mapping))

	t, err = xmldom.ParseXML(bytes.NewReader(template))
	if err != nil {
		return nil, err
	}

	in, err = xmldom.ParseXML(sf)
	if err != nil {
		return nil, err
	}

	if ifClause != "" {
		ifPath, err := xpath.Compile(ifClause)
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
	} else if err == io.EOF {
		log("End of File")
	}

	return []byte(t.XML()), nil
}

type SortKey struct {
	Asc bool
	Key string
}

type SortKeys struct {
	Keys []SortKey
	Node *xmldom.Node
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

func IterNodes(xp *xpath.Expr, n *xmldom.Node) []*xmldom.Node {
	var nodes []*xmldom.Node
	res := xp.Evaluate(n)
	switch res.(type) {
	case string:
		nodes = append(nodes, n.OwnerDocument().CreateTextNode(res.(string)))
	case *xpath.Iterator:
		i := res.(*xpath.Iterator)
		for i.MoveNext() {
			nodes = append(nodes, i.Current())
		}
	default:
		nodes = append(nodes, n.OwnerDocument().CreateTextNode(fmt.Sprintf("%v", res)))
	}
	return nodes
}

// curdir:   directory where the template file is
// src:      data source relative to curdir (empty denotes the file being
//           templated)
// p:        parser for the mapping markup (the content of <template-instance/>)
// in:       DOM for the data source (src)
// tmpl:     DOM for the template markup (the content of the <template/> tag)
// sortk:    Sort key list for collections
func runTemplate(curdir, src string, p *parser.Parser, in *xmldom.Node, tmpl *xmldom.Node, sortk *SortKeys) error {
	logIndent()
	defer logDeIndent()
	depth := p.Depth()
	//ownerdoc := xmldom.NewDocument()

	log("\n[%d] Templating in file %#v\nfrom source data: %#v\nusing template: %#v\n\n", depth, string(src), in.XML(), tmpl.XML())

	for {
		p.End()
		if p.Depth() < depth {
			log("\n[%d] Done Templating %#v\nresult: %#v\n\n", depth, src, string(tmpl.XML()))
			return nil
		}

		err := p.Next()
		if err != nil {
			//log("[%d] Error %v Templating %#v\nresult: %#v\n\n", depth, err, src, tmpl.XML())
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

			path, err := xpath.CompileNS(pathStr, namespaces)
			if err != nil {
				return err
			}

			nodes := IterNodes(path, in)

			if format != "" {
				nodes, err = formatNodes(in.OwnerDocument(), curdir, src, format, nodes, p)
				if err != nil {
					return err
				}
			}

			s.Key = string(nodesToText(nodes))

			if s.Key != "" {
				sortk.Keys = append(sortk.Keys, s)
			}

		} else if p.IsStartTag() && p.Data() == "map" {
			log("\n[%d] Mapping: %v\n", depth, string(p.Token().String()))
			var frompath, topath *xpath.Expr
			from := p.Attr("from")
			to := p.Attr("to")
			dataattr := p.Attr("data")
			format := p.Attr("format")
			multi := p.Attr("multiple")
			fetch := p.Attr("fetch")
			onlyif := p.Attr("only-if")

			log("[%d]   source context:   %v\n", depth, in.XML())
			log("[%d]   template context: %v\n", depth, tmpl.XML())

			if to != nil {
				topath, err = xpath.CompileNS(to.Val, namespaces)
				if err != nil {
					return err
				}
			}

			if onlyif != nil && onlyif.Val == "empty" && topath != nil {
				i := topath.EvaluateNode(tmpl)
				empty := true
				for i.Next() {
					if path_children.Exists(i.Node()) {
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
				frompath, err = xpath.CompileNS(from.Val, namespaces)
				if err != nil {
					return err
				}
			}

			var nodes []*xmldom.Node

			if nodes == nil && frompath != nil {
				log("[%d]   frompath: %s\n", depth, from.Val)
				//log("  frompath: %#v\n", string(in.XML()))
				i := IterNodes(frompath, in)
				for j, n := range i {
					nodes = append(nodes, n)
					log("[%d]   %d --> %#v\n", j, depth, string(n.XML()))
				}
				log("[%d]   frompath: %s (%d results)\n", depth, from.Val, len(nodes))
			}

			if nodes == nil && dataattr != nil && dataattr.Val == "relative-url" {
				n := in.OwnerDocument().CreateTextNode(src)
				nodes = append(nodes, n)
			} else if nodes == nil && dataattr != nil && dataattr.Val == "relative-dir" {
				n := in.OwnerDocument().CreateTextNode(filepath.Dir(src) + "/")
				nodes = append(nodes, n)
			}

			if format != nil {
				nodes, err = formatNodes(in.OwnerDocument(), curdir, src, format.Val, nodes, p)
				if err != nil {
					return err
				}
				log("[%d]   format %s:\n", depth, format.Val)
				for i, n := range nodes {
					log("[%d]   %d --> %s\n", depth, i, n.XML())
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

					n := tmpl.CloneNode(true)
					var sort2 SortKeys
					err = func() error {
						sf, err := os.Open(newsrcfile)
						if err != nil {
							return err
						}
						defer sf.Close()
						in, err := xmldom.ParseXML(sf)
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
					log("[%d] Resource %d/%d %#v templating result: %#v\n", depth, i+1, len(newsrcs), newsrc, n.XML())
				}
				sort.Stable(ByKey(sortedNodes))

				var res []*xmldom.Node
				for _, n := range sortedNodes {
					res = append(res, n.Node)
				}
				err = ReplaceInner(tmpl, res)
				if err != nil {
					panic(err)
				}
				log("[%d] Resource templating result: %#v\n", depth, tmpl.XML())
				nodes = nil
			} else if topath != nil && nodes != nil {
				// FIXME: set xml:base
				log("[%d] evaluate topath=%s, %s\n", depth, to.Val, topath.DebugString())
				log("[%d] multiple=%v\n", depth, multi)
				matches := topath.EvaluateNode(tmpl).Nodes()
				log("[%d] %d to matches: %#v\n", depth, len(matches), to.Val)
				log("[%d] . in template: %s\n", depth, tmpl.XML())
				for _, tnode := range matches {

					if multi != nil && multi.Val == "true" {
						log("[%d] Multiple (%d) templating of %#v\n", depth, len(nodes), tnode.XML())
						submap, err := p.RawContent()
						if err != nil {
							return err
						}
						var sortedNodes []SortKeys
						for i, inode := range nodes {
							n := tnode.CloneNode(true)
							//log("Insert %#v\n", string(n.Node.XML()))
							//log(" before %#v\n", string(tnode.Node.XML()))
							pp := parser.NewParser(bytes.NewReader(submap))
							var sort2 SortKeys
							err = runTemplate(curdir, src, pp, inode, n, &sort2)
							if err != nil && err != io.EOF {
								log("[%d] Multiple templating error %v\n", depth, err)
								return err
							}
							sort2.Node = n
							sortedNodes = append(sortedNodes, sort2)
							log("[%d] Multiple templating result %d: %#v\n", depth, i, n.XML())
						}
						sort.Stable(ByKey(sortedNodes))
						for _, n := range sortedNodes {
							nn := n.Node.CloneNode(true)
							err := tnode.OwnerDocument().ImportNode(nn)
							if err != nil {
								panic(err)
							}
							_, err = tnode.ParentNode().InsertBefore(nn, tnode)
							if err != nil {
								panic(err)
							}
						}
						_, err = tnode.ParentNode().RemoveChild(tnode)
						if err != nil {
							panic(err)
						}

					} else if tnode.NodeType() == xmldom.ElementNode {
						log("[%d] Set %d children\n", depth, len(nodes))
						var children []*xmldom.Node
						for i := range nodes {
							log("[%d] %d --> %#v\n", i, depth, nodes[i])
							log("[%d] %d --> %#v\n", i, depth, nodes[i].XML())
							children = append(children, nodes[i])
						}
						err := ReplaceInner(tnode, children)
						if err != nil {
							panic(err)
						}
						log("[%d] ==> %#v\n", depth, tnode.XML())

					} else {
						log("[%d] Convert %d nodes to text\n", depth, len(nodes))
						tnode.SetNodeValue(string(nodesToText(nodes)))
					}

				}
				log("\n[%d] Maping Result: %#v\n", depth, tmpl.XML())
			} else {
				log("\n[%d] Mapping Aborted\n", depth)
			}
		} else if p.IsStartTag() || p.IsEndTag() {
			log("[%d] %s", depth, p.Token().String())
		} else {
			log("[%d] %#v", depth, p.Token().String())
		}
		//log("[%d] Template result: %v", depth, tmpl.XML())
	}
}

func ReplaceInner(n *xmldom.Node, newChildren []*xmldom.Node) error {
	for n.FirstChild() != nil {
		_, err := n.RemoveChild(n.FirstChild())
		if err != nil {
			return err
		}
	}
	for _, cn := range newChildren {
		cn = cn.CloneNode(true)
		err := n.OwnerDocument().ImportNode(cn)
		if err != nil {
			panic(err)
		}
		_, err = n.AppendChild(cn)
		if err != nil {
			return err
		}
	}
	return nil
}

type AttrsInterface interface {
	AttrVal(name, defVal string) string
}

func formatNodes(ownerdoc *xmldom.Node, curdir, src, format string, nodes []*xmldom.Node, attrs AttrsInterface) ([]*xmldom.Node, error) {
	switch format {
	default:
		log("   unknown format %#v, aborting mapping\n", format)
		nodes = nil
		break
	case "text":
		nodes = []*xmldom.Node{ownerdoc.CreateTextNode(string(nodesToText(nodes)))}
		break
	case "split":
		text := string(nodesToText(nodes))
		nodes = nil
		for _, txt := range strings.Split(text, " ") {
			nodes = append(nodes, ownerdoc.CreateTextNode(txt))
		}
		break
	case "debug":
		nodes = []*xmldom.Node{ownerdoc.CreateTextNode(string("DEBUG[" + nodesToText(nodes) + "]"))}
		break
	case "link-relative":
		data, err := relurl.UrlJoinString(filepath.Dir(src), string(nodesToText(nodes)), curdir)
		if err != nil {
			return nil, err
		}
		log("   convert to relative link: %#v\n", string(data))
		nodes = []*xmldom.Node{ownerdoc.CreateTextNode(data)}
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
		nodes = []*xmldom.Node{ownerdoc.CreateTextNode(data)}
		break
	}
	return nodes, nil
}

func nodesToText(nodes []*xmldom.Node) string {
	var data string
	for _, inode := range nodes {
		//log("text for %s: %s\n", string(inode.Node.XML()), string(inode.Node.String()))
		data += inode.AsText()
	}
	return data
}

func nodesToSlice(nodes []*xmldom.Node) []string {
	var data []string
	for _, inode := range nodes {
		data = append(data, inode.AsText())
	}
	return data
}

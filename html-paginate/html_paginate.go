package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/mildred/htmltools/parser"
	_ "github.com/mildred/htmltools/relurl"
	"io"
	"io/ioutil"
	"launchpad.net/xmlpath"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	name := flag.String("n", "", "File name")
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
		if *name == "" {
			*name = infile
		}
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

	if *name == "" {
		fmt.Fprintln(os.Stderr, "html-paginate: when working with stdin, you need to provide a filename with -n.")
		os.Exit(1)
	}

	err := handleTags(dir, filepath.Base(*name), f1, os.Stdout)
	if err != nil && err != io.EOF {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	os.Exit(0)
}

type Pagination struct {
	InPath   *xmlpath.Path
	ForPath  *xmlpath.Path
	MetaHead *xmlpath.Path
	FileName string
	PageSize int
}

func readPagination(r io.Reader) (w *os.File, pagination Pagination, err error) {
	// Prepare copy of file without the pagination tag
	w, err = ioutil.TempFile("", "html-paginate.temp.html")
	if err != nil {
		return
	}
	defer os.Remove(w.Name())
	defer w.Seek(0, 0)

	p := parser.NewParser(r)
	for {

		err = p.Next()
		if err == io.EOF {
			err = nil
			return
		} else if err != nil {
			return
		}

		var raw []byte = p.Raw()

		if p.IsStartTag() && p.Data() == "pagination" {

			forPath := p.AttrVal("for", "")
			inPath := p.AttrVal("in", "")
			metaHead := p.AttrVal("head", "")
			filename := p.AttrVal("filename", "${basename}.${pagenum}.${ext}")
			pageSize, _ := strconv.Atoi(p.AttrVal("size", "10"))
			if pageSize <= 0 {
				pageSize = 10
			}

			if forPath == "" {
				err = fmt.Errorf("<pagination /> with empty for attribute")
				return
			}

			if inPath == "" {
				err = fmt.Errorf("<pagination /> with empty in attribute")
				return
			}

			if pagination.ForPath != nil {
				err = fmt.Errorf("More than one <pagination />")
				return
			}

			pagination = Pagination{
				FileName: filename,
				PageSize: pageSize,
			}

			pagination.ForPath, err = xmlpath.Compile(forPath)
			if err != nil {
				return
			}

			pagination.InPath, err = xmlpath.Compile(inPath)
			if err != nil {
				return
			}

			if metaHead != "" {
				pagination.MetaHead, err = xmlpath.Compile(metaHead)
				if err != nil {
					return
				}
			}

			raw = nil
		}

		if raw != nil {
			_, err = w.Write(raw)
			if err != nil {
				return
			}
		}

	}
}

func handleTags(curdir, curfile string, r io.Reader, w io.Writer) error {
	var err error
	var r2 *os.File
	var pagination Pagination
	var in *xmlpath.Node

	r2, pagination, err = readPagination(r)
	if r2 != nil {
		defer r2.Close()
	}
	if err != nil {
		return err
	} else if pagination.ForPath == nil {
		_, err = io.Copy(w, r2)
		return err
	}

	in, err = xmlpath.ParseHTML(r2)
	if err != nil {
		return err
	}

	nodes := pagination.ForPath.Iter(in).Nodes()
	pages, lastPage := computePages(pagination.PageSize, len(nodes))

	for pageidx, page := range pages {
		log("Page %d contains %v\n", pageidx+1, page)

		in2 := in.Copy().Ref
		fname := expandFileName(fileNameArgs(curfile, pagination.FileName, pageidx))
		createPage(in2, pagination.ForPath, pagination.InPath, page, PageMeta{
			index:    pageidx,
			size:     len(pages),
			srcfile:  curfile,
			template: pagination.FileName,
			head:     pagination.MetaHead,
		})
		//log("Page %d: %#v\n", pageidx+1, string(in2.Node.XML()))

		err = func() error {
			logv("Create page %s\n", fname)
			fname = filepath.Join(curdir, fname)

			f, err := os.Create(fname)
			if err != nil {
				return err
			}
			defer f.Close()

			_, err = f.Write(in2.Node.XML())
			return err
		}()

		if err != nil {
			return err
		}
	}

	log("Last page contains %v\n", lastPage)
	inref := in.Ref
	createPage(inref, pagination.ForPath, pagination.InPath, lastPage, PageMeta{
		index:    -1,
		size:     len(pages),
		srcfile:  curfile,
		template: pagination.FileName,
		head:     pagination.MetaHead,
	})
	_, err = w.Write(inref.Node.XML())
	return err
}

type TemplateArgs struct {
	template string
	basename string
	ext      string
	num      string
	idx      string
}

func fileNameArgs(srcfile, template string, index int) TemplateArgs {
	var args TemplateArgs
	args.template = template
	args.ext = filepath.Ext(srcfile)
	args.basename = srcfile[0 : len(srcfile)-len(args.ext)]
	args.num = strconv.Itoa(index + 1)
	args.idx = strconv.Itoa(index)
	if len(args.ext) > 0 && args.ext[0] == '.' {
		args.ext = args.ext[1:]
	}
	return args
}

func expandFileName(a TemplateArgs) string {
	fname := a.template
	fname = strings.Replace(fname, "${basename}", a.basename, -1)
	fname = strings.Replace(fname, "${ext}", a.ext, -1)
	fname = strings.Replace(fname, "${num}", a.num, -1)
	fname = strings.Replace(fname, "${idx}", a.idx, -1)
	return fname
}

func computePages(pageSize, numItems int) (pages [][]int, lastPage []int) {
	var curPage []int
	for i := 0; i < numItems; i++ {
		curPage = append(curPage, i)
		if len(curPage) >= pageSize {
			pages = append(pages, curPage)
			curPage = nil
		}
	}
	if len(curPage) > 0 {
		pages = append(pages, curPage)
	}
	for i := numItems - 1; i >= 0; i-- {
		lastPage = append(lastPage, i)
		if len(lastPage) >= pageSize {
			break
		}
	}
	return
}

type PageMeta struct {
	index    int // negative if this is the last page
	size     int
	srcfile  string
	template string
	head     *xmlpath.Path
}

func createPage(in *xmlpath.NodeRef, forPath, inPath *xmlpath.Path, page []int, meta PageMeta) {
	var insertionPoint *xmlpath.NodeRef
	inIt := inPath.Iter(in.Node)
	nodes := forPath.Iter(in.Node).Nodes()

	if inIt.Next() {
		insertionPoint = inIt.Node().Ref
	} else if len(nodes) > 0 {
		insertionPoint = nodes[0].Node.Parent().Ref
	}
	if insertionPoint == nil {
		return
	}

	for _, node := range nodes {
		node.Node.Remove()
	}
	//log("insertion point: %#v\n", string(insertionPoint.Node.XML()))
	for _, i := range page {
		insertionPoint.Node.InsertLastChild(nodes[i].Node)
		//log("\nitem %d: %#v\n", i, string(nodes[i].Node.XML()))
		//log("insertion point %d: %#v\n", i, string(insertionPoint.Node.XML()))
	}

	var head *xmlpath.NodeRef
	if meta.head != nil {
		log("head: ok\n")
		it := meta.head.Iter(in.Node)
		if it.Next() {
			head = it.Node().Ref
		}
	}
	if head != nil {
		args := fileNameArgs(meta.srcfile, meta.template, meta.index)
		head.Node.InsertLastChild(metaNode("pagination", "true"))
		if meta.index < 0 {
			head.Node.InsertLastChild(metaNode("pagination.latest", "latest"))
		} else {
			head.Node.InsertLastChild(metaNode("pagination.latest", ""))
		}
		log("head: %v\n", string(head.Node.XML()))
		head.Node.InsertLastChild(metaNode("pagination.pageidx", strconv.Itoa(meta.index)))
		head.Node.InsertLastChild(metaNode("pagination.pagenum", strconv.Itoa(meta.index+1)))
		head.Node.InsertLastChild(metaNode("pagination.size", strconv.Itoa(meta.size)))
		head.Node.InsertLastChild(metaNode("pagination.template", meta.template))
		head.Node.InsertLastChild(metaNode("pagination.template.num", args.num))
		head.Node.InsertLastChild(metaNode("pagination.template.idx", args.idx))
		head.Node.InsertLastChild(metaNode("pagination.template.basename", args.basename))
		head.Node.InsertLastChild(metaNode("pagination.template.ext", args.ext))
		var pages_num []string
		var pages_idx []string
		for i := 0; i < meta.size; i++ {
			pages_num = append(pages_num, strconv.Itoa(i+1))
			pages_idx = append(pages_idx, strconv.Itoa(i))
		}
		head.Node.InsertLastChild(metaNode("pagination.pages.idx", strings.Join(pages_idx, " ")))
		head.Node.InsertLastChild(metaNode("pagination.pages.num", strings.Join(pages_num, " ")))
		for i := 0; i < meta.size; i++ {
			a := args
			a.idx = strconv.Itoa(i)
			a.num = strconv.Itoa(i + 1)
			head.Node.InsertLastChild(linkNode("rel",
				fmt.Sprintf("pagination.page.idx.%d", i),
				expandFileName(a)))
			head.Node.InsertLastChild(linkNode("rel",
				fmt.Sprintf("pagination.page.num.%d", i+1),
				expandFileName(a)))
		}
		head.Node.InsertLastChild(linkNode("rel", "pagination.page.latest", meta.srcfile))
	}
}

func metaNode(name, content string) *xmlpath.Node {
	n, err := xmlpath.Parse(bytes.NewReader([]byte(fmt.Sprintf("\t<meta name=\"%s\" content=\"%s\" />\n\t", htmlEncode(name), htmlEncode(content)))))
	if err != nil {
		panic(err)
	}
	return n
}

func linkNode(relrev, relrevval, href string) *xmlpath.Node {
	n, err := xmlpath.Parse(bytes.NewReader([]byte(fmt.Sprintf("\t<link %s=\"%s\" href=\"%s\" />\n\t", relrev, htmlEncode(relrevval), htmlEncode(href)))))
	if err != nil {
		panic(err)
	}
	return n
}

func htmlEncode(str string) string {
	str = strings.Replace(str, `&`, `&amp;`, -1)
	str = strings.Replace(str, `>`, `&gt;`, -1)
	str = strings.Replace(str, `<`, `&lt;`, -1)
	str = strings.Replace(str, `'`, `&#39;`, -1)
	str = strings.Replace(str, `"`, `&quot;`, -1)
	return str
}

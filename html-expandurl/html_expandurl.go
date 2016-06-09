package main

import (
	"flag"
	"fmt"
	"github.com/mildred/htmltools/relurl"
	"golang.org/x/net/html"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	flag.Parse()
	chdir := flag.String("C", "", "Change directory before operation")
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

	err := handleTags(filepath.Dir(infile), infile, os.Stdout)
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
	}

	abscurdir, err := filepath.Abs(curdir)
	if err != nil {
		return err
	}

	z := html.NewTokenizer(f1)
	bases := []string{}

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
			changed := false
			xmlBase := ""
			newAttrs := []html.Attribute{}
			for _, a := range t.Attr {
				if a.Key == "xml:base" {
					xmlBase = a.Val
					changed = true
				} else {
					newAttrs = append(newAttrs, a)
				}
			}

			//if xmlBase != "" {
			//	fmt.Fprintf(os.Stderr, "xml:base=\"%v\" ...\n", xmlBase)
			//}

			t.Attr = newAttrs

			if len(bases) > 0 && bases[len(bases)-1] != "" {
				if xmlBase == "" {
					xmlBase = bases[len(bases)-1]
				} else {
					xmlBase = filepath.Join(bases[len(bases)-1], xmlBase)
				}
				absBase := xmlBase
				if !filepath.IsAbs(absBase) {
					absBase = filepath.Join(abscurdir, absBase)
				}
				xmlBase, err = filepath.Rel(abscurdir, absBase)
				if err != nil {
					return err
				}
			}
			bases = append(bases, xmlBase)
			//fmt.Fprintf(os.Stderr, "%v +> %#v\n", t.String(), bases)

			baseUrl, err := url.Parse(xmlBase)

			if err == nil {
				//fmt.Fprintf(os.Stderr, "%v %#v\n", t.String(), baseUrl.String())
				for i, a := range t.Attr {
					u := getURL(t.Data, a.Key, a.Val)
					if u != nil && !u.IsAbs() && !strings.HasPrefix(a.Val, "//") {
						//u2 := filepath.Join(xmlBase, u.String())
						//var u2 string
						//if u.Path == "" {
						//	u2 = u.String()
						//} else {
						//u2 := baseUrl.ResolveReference(u).String()
						//}
						/*
							if u.Path != "" {
								if u.Scheme == "" {
									u.Scheme = baseUrl.Scheme
								}
								if u.User == nil {
									u.User = baseUrl.User
								}
								if u.Host == "" {
									u.Host = baseUrl.Host
								}
								if !filepath.IsAbs(u.Path) {
									u.Path = filepath.Join(baseUrl.Path, u.Path)
								}
								if u.RawQuery == "" {
									u.RawQuery = baseUrl.RawQuery
								}
								if u.Fragment == "" {
									u.Fragment = baseUrl.Fragment
								}
							}
							u2 := u.String()
						*/
						u2, err := relurl.UrlJoin(baseUrl, u, abscurdir)
						if err != nil {
							return err
						}
						if u2 != a.Val {
							//fmt.Fprintf(os.Stderr, "%v %#v - %#v => %#v\n", t.String(), baseUrl.String(), u.String(), u2)
							//if len(a.Val) > 0 && a.Val[len(a.Val)-1] == '/' && (len(u2) == 0 || u2[len(u2)-1] != '/') {
							if strings.HasSuffix(a.Val, "/") && !strings.HasSuffix(u2, "/") {
								u2 += "/"
							}
							t.Attr[i].Val = u2
							changed = true
						}
					}
				}
			}

			if changed {
				rawData = []byte(t.String())
			}
		}

		_, err := f2.Write(rawData)
		if err != nil {
			return err
		}

		if tt == html.EndTagToken || tt == html.SelfClosingTagToken {
			bases = bases[:len(bases)-1]
			//fmt.Fprintf(os.Stderr, "%v -> %#v\n", string(rawData), bases)
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

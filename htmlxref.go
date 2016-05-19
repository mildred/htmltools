package main

import (
	"flag"
	"fmt"
	"golang.org/x/net/html"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	flag.Parse()
	fname := flag.Arg(0)
	err := xref(fname)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func readAttributes(z *html.Tokenizer, attrs bool) (attributes [][]string, direction, kind string, href string) {
	for attrs {
		var key, val []byte
		key, val, attrs = z.TagAttr()
		attributes = append(attributes, []string{string(key), string(val)})
		if string(key) == "href" {
			href = string(val)
		} else if string(key) == "rev" {
			direction = "rev"
			kind = string(val)
		} else if string(key) == "rel" {
			direction = "rel"
			kind = string(val)
		}
	}
	return
}

func reverse(direction string) string {
	switch direction {
	case "rel":
		return "rev"
	case "rev":
		return "rel"
	default:
		panic("Incorrect direction")
	}
}

func xref(fname string) error {
	f, err := os.Open(fname)
	if err != nil {
		return err
	}
	defer f.Close()

	z := html.NewTokenizer(f)
	z.AllowCDATA(true)

	var breadcrumb []string
	for {
		tk := z.Next()
		if tk == html.ErrorToken {
			err := z.Err()
			if err != io.EOF {
				return err
			}
			break
		}
		if tk == html.StartTagToken || tk == html.SelfClosingTagToken {
			tagName, attrs := z.TagName()
			breadcrumb = append(breadcrumb, string(tagName))

			if string(tagName) == "link" {
				_, direction, kind, href := readAttributes(z, attrs)
				fmt.Printf("Link: %v=%v %v\n", direction, kind, href)

				if kind != "" {
					target := filepath.Join(filepath.Dir(fname), href)
					err := ensure_link(fname, target, reverse(direction), kind)
					if err != nil && os.IsNotExist(err) {
						fmt.Printf("Could not modify: %s\n", href)
					} else {
						return err
					}
				}
			}
		}
		if tk == html.EndTagToken || tk == html.SelfClosingTagToken {
			breadcrumb = breadcrumb[:len(breadcrumb)-1]
		}
	}
	return nil
}

func ensure_link(target, source, direction, kind string) error {
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()

	f2, err := ioutil.TempFile(filepath.Dir(source), filepath.Base(target))
	if err != nil {
		return err
	}
	defer f2.Close()

	z := html.NewTokenizer(f)
	var breadcrumb []string
	var lastText string
	for {
		tk := z.Next()
		raw0 := z.Raw()
		rawData := make([]byte, len(raw0))
		copy(rawData, raw0)
		if tk == html.ErrorToken {
			err := z.Err()
			if err != io.EOF {
				e := os.Remove(f2.Name())
				if e != nil {
					fmt.Fprintln(os.Stderr, e.Error())
				}
				return err
			}
			break
		}
		if tk == html.StartTagToken || tk == html.SelfClosingTagToken {
			tagName, attrs := z.TagName()
			breadcrumb = append(breadcrumb, string(tagName))

			if string(tagName) == "link" {
				_, direction2, kind2, href := readAttributes(z, attrs)
				target2 := filepath.Join(filepath.Dir(source), href)
				//fmt.Printf("Link: %v\n", attributes)

				if direction2 == direction && kind2 == kind && samePath(target, target2) {
					// The link is already there
					return os.Remove(f2.Name())
				}
			}
		}
		if tk == html.TextToken {
			lastText = string(z.Text())
		}
		if tk == html.EndTagToken || tk == html.SelfClosingTagToken {
			tagName, _ := z.TagName()
			if string(tagName) == "head" {
				href, err := filepath.Rel(filepath.Dir(source), target)
				if err != nil {
					e := os.Remove(f2.Name())
					if e != nil {
						fmt.Fprintln(os.Stderr, e.Error())
					}
					return err
				}
				indent := detectIndent(lastText, 1)
				linkTag := fmt.Sprintf("%s<link %s=\"%s\" href=\"%s\" />\n%s",
					indent,
					direction, html.EscapeString(kind), html.EscapeString(href),
					indent)
				_, err = f2.Write([]byte(linkTag))
				if err != nil {
					e := os.Remove(f2.Name())
					if e != nil {
						fmt.Fprintln(os.Stderr, e.Error())
					}
					return err
				}
			}

			breadcrumb = breadcrumb[:len(breadcrumb)-1]
		}

		_, err = f2.Write(rawData)
		if err != nil {
			e := os.Remove(f2.Name())
			if e != nil {
				fmt.Fprintln(os.Stderr, e.Error())
			}
			return err
		}
	}

	return os.Rename(f2.Name(), f.Name())
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

func samePath(path1, path2 string) bool {
	st1, err1 := os.Stat(path1)
	st2, err2 := os.Stat(path2)
	if err1 != nil || err2 != nil {
		return false
	}
	return os.SameFile(st1, st2)
}

html-markdown
=============

Introduction
------------

html-markdown is a tool that takes an html input with a special `<markdown/>`
tag, evaluates the markdown contained within that tag, and replaces it in its
output with the evaluated markup. It is included in the family of htmltools that
evaluates statically custom HTML tags offline.

Those tools are written in Go, and the present document explains how
html-markdown is operating.

Command-line interface
----------------------

First, let's look at how html-markdown reacts to command-line arguments. There
is not a lot of things it does, when parsing command-line. it basically only
waits for a single optional argument on the CLI.

(main)
```go
package main

import (
	"flag"
	"fmt"
	// ...other imports...
)

func main() {
	flag.Parse()
	infile := flag.Arg(0)

	if infile == "-" {
		infile = ""
	}

	// ...readfile...
	// ...parsefile...

	os.Exit(0)
}
```

In the previous code snippet, we see that we fetch a single command-line
argument, and assign it to the `infile` variable. if the argument cntains `-` as
it is standard to denote standard input, the variable is set to the empty
string, the same value that the variable would contain if no argument was passed
to the CLI.

Hence, if the argument is `-` or the empty string, it will parse the standard
input, else it will parse the named file.

To read files, we need some imports first:

(other imports)
```go
	"io"
	"os"
	"path/filepath"
```

When reading the input, we need to get a file object and we also need to keep
track of the directory where the input file is, to handle relative paths. In
case the standard input is used as a file source, the current working directory
serves as the base directory.

(readfile)
```go
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
```

HTML Tags processing
--------------------

Next, after command-line arguments are parsed, comes the tag processing. For
that we are using the htmltools parser. Here are the imports related to it (the
parser and the data structure module it needs):

(other imports)
```go
	"github.com/mildred/htmltools/parser"
	"golang.org/x/net/html"
```

The parsing routine is split in a separate function where we define `f1` and
`f2` as the input file and output file respectively. The parsing and generation
of the output is simultaneous. This is because most HTML tags are parsed and
written as-is without transformation. Only the markdown tag will need to be
processed.

(main)
```go
func handleTags(curdir string, f1 io.Reader, f2 io.Writer) error {
	p := parser.NewParser(f1)

	for {
		// ...handletags...
	}
}
```

First, let's parse the token in the loop, and handle parsing errors:

(handletags)
```go
		err := p.Next()
		if err != nil {
			return err
		}
```

Then, let's handle the general case, read the tag and write it unmodified to the
standard output:

(handletags)
```go
		var raw []byte = p.Raw()

		// ...custom handling...

		if raw != nil {
			_, err = f2.Write(raw)
			if err != nil {
				return err
			}
		}
```

The raw bytes corresponding to the tag are fetched from the parser, and put to
the output file as it is. Any custom handling goes in between to modify or empty
the raw bytes. Any write error is handled too.

Next, let's see the condition to handle the markdown tag:

(custom handling)
```go
		if p.Type() == html.StartTagToken && p.Data() == "markdown" {
			// ...handle markdown...
		}
```

We handle the markdown tag if and only if:

- the token is a start tag token
- the tag name is `markdown`

Finally, to make use of the parsing code, it must be hooked up in the main
routine. We used a separate routine to be able to write once the error handling
code and exit handling.

(parsefile)
```go
	err := handleTags(dir, f1, os.Stdout)
	if err != nil && err != io.EOF {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
```

Here, we specify the output to the standard output, and handle errors (other
than end of file which is normal to happen) by printing the error message and
terminating the program with a non zero exit status.

Handle markdown
---------------

To handle markdown, we make use of the following import

(other imports)
```go
	"github.com/golang-commonmark/markdown"
```

To handle the markdown tag, we need to:

- read the tag content
- parse the markdown and generate HTML markup
- feed back the markup to the output file

The parsing library allows is to simply read the tag content:

(handle markdown)
```go
			data, err := p.TextContent()
			if err != nil {
				return err
			}
			//fmt.Fprintf(os.Stderr, "markdown %#v\n", string(data))
```

Here, a disabled line allows us to print the raw markdown markup prior to
handling in the markdown library.

Then, we make use of the markdown library to convert the markdown code and feed
it back to the `raw` variable as seen in the previous title:

(handle markdown)
```go
			md := markdown.New(markdown.XHTMLOutput(true))
			raw = []byte(md.RenderToString(data))
```


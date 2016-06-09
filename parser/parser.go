package parser

import (
	"github.com/mildred/htmltools/htmldepth"
	"golang.org/x/net/html"
	"io"
)

type Parser struct {
	z    *html.Tokenizer
	t    *html.Token
	raw  []byte
	d    *htmldepth.HTMLDepth
	open bool
}

func NewParser(f io.Reader) *Parser {
	return &Parser{
		z:    html.NewTokenizer(f),
		t:    nil,
		raw:  nil,
		d:    &htmldepth.HTMLDepth{},
		open: false,
	}
}

// Go to next token
func (p *Parser) Next() error {
	err := p.End()
	if err != nil {
		return err
	}

	tt := p.z.Next()
	if tt == html.ErrorToken {
		return p.z.Err()
	}

	raw0 := p.z.Raw()
	p.raw = make([]byte, len(raw0))
	copy(p.raw, raw0)
	t := p.z.Token()
	p.t = &t

	if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
		p.d.Start(string(p.t.Data))
	}

	p.open = true

	return nil
}

// End current token
func (p *Parser) End() error {
	if p.open {
		p.open = false
		if p.t.Type == html.EndTagToken || p.t.Type == html.SelfClosingTagToken {
			err := p.d.Stop(string(p.t.Data))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Return the current token
func (p *Parser) Token() *html.Token {
	return p.t
}

// Return the token type
func (p *Parser) Type() html.TokenType {
	return p.t.Type
}

// Return the token data (tag name, text content, ...)
func (p *Parser) Data() string {
	return string(p.t.Data)
}

// Return true if it is a start tag or self closing tag
func (p *Parser) IsStartTag() bool {
	return p.t.Type == html.StartTagToken || p.t.Type == html.SelfClosingTagToken
}

// Return true if it is a start tag or self closing tag
func (p *Parser) IsEndTag() bool {
	return p.t.Type == html.EndTagToken || p.t.Type == html.SelfClosingTagToken
}

// Return the named attribute
func (p *Parser) Attr(name string) *html.Attribute {
	return Attr(p.t, name)
}

// Return the named attribute
func (p *Parser) AttrVal(name, defval string) string {
	a := Attr(p.t, name)
	if a == nil {
		return defval
	} else {
		return a.Val
	}
}

// Return the named attribute
func Attr(t *html.Token, name string) *html.Attribute {
	for _, a := range t.Attr {
		if a.Key == name {
			return &a
		}
	}
	return nil
}

// Return the byte representation of the current token (unmodified)
func (p *Parser) Raw() []byte {
	return p.raw
}

// Return the current breadcrumb
func (p *Parser) Breadcrumb() []string {
	return p.d.Breadcrumb
}

// Current depth
func (p *Parser) Depth() int {
	return len(p.d.Breadcrumb)
}

// On a start tag, skip until the end tag and return the raw content
// The parser is left after the end tag token, but after is has closed (End has
// been called and the breadcrumb excludes the end tag)
func (p *Parser) RawContent() ([]byte, error) {
	if p.t.Type != html.StartTagToken {
		return nil, nil
	}

	var data []byte
	var depth int = p.d.Depth()

	for {
		err := p.Next()
		if err != nil {
			return nil, err
		}

		err = p.End()
		if err != nil {
			return nil, err
		}

		if depth > p.d.Depth() {
			return data, nil
		}

		data = append(data, p.Raw()...)
	}
}

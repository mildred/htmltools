package xmlpath

import (
	"encoding/xml"
	"fmt"
	"io"
)

// Node is an item in an xml tree that was compiled to
// be processed via xml paths. A node may represent:
//
//     - An element in the xml document (<body>)
//     - An attribute of an element in the xml document (href="...")
//     - A comment in the xml document (<!--...-->)
//     - A processing instruction in the xml document (<?...?>)
//     - Some text within the xml document
//
type Node struct {
	// Node Kind
	kind NodeKind

	// Tag name for start nodes
	// attribute name for attributes
	// empty for others (including end node)
	name xml.Name

	// Attribute value for attributes
	attr string

	// Text content for text nodes, comments and processing instructions
	text []byte

	// List of all nodes in the document
	nodes []Node

	// Index of the current node in the `nodes' list
	pos int

	// Index of the paired node (for begin and end tags)
	end int

	// Parent node
	up *Node

	// For start node, the list of direct children
	down []*Node

	// Persistent pointer to the node itself
	Ref *NodeRef
}

type NodeRef struct {
	Node *Node
}

type NodeKind int

const (
	AnyNode NodeKind = iota
	StartNode
	EndNode
	AttrNode
	TextNode
	CommentNode
	ProcInstNode
)

func (n *Node) Parent() *Node {
	return n.up
}

func (n *Node) InsertFirstChild(cn *Node) {
	if len(n.down) == 0 {
		n.ReplaceInner(*cn)
	} else {
		n.down[0].InsertBefore(*cn)
	}
}

func (n *Node) InsertLastChild(cn *Node) {
	if len(n.down) == 0 {
		n.ReplaceInner(*cn)
	} else {
		n.down[len(n.down)-1].InsertAfter(*cn)
	}
}

func (n *Node) Copy() *Node {
	nodes := append([]Node{}, n.extract()...)
	for i := range nodes {
		nodes[i].Ref = nil
		nodes[i].text = append([]byte{}, nodes[i].text...)
	}
	refresh(nodes)
	return nodes[0].Ref.Node
}

func (n *Node) Kind() NodeKind {
	return n.kind
}

// Format to XML
func (n *Node) XML() []byte {
	if n.up == nil {
		return n.toXML(nil)
	} else {
		return n.toXML(n.up.FindNamespaces())
	}
}

func findNS(ns map[string]string, nsuri string) string {
	res := ""
	for k, v := range ns {
		if v == nsuri {
			return k
		}
		//res += "[" + k + "=" + v + "]?"
	}
	//res += nsuri
	return res
}

func (n *Node) FindNamespaces() map[string]string {
	var ns map[string]string = nil
	if n.up != nil {
		ns = n.up.FindNamespaces()
	}
	return n.namespaces(ns)
}

func (n *Node) namespaces(ns map[string]string) map[string]string {
	if n.kind != StartNode {
		return ns
	}

	var res map[string]string = map[string]string{
		"xml": "http://www.w3.org/XML/1998/namespace",
		"":    "",
	}
	if ns != nil {
		for k, v := range ns {
			res[k] = v
		}
	}

	for i := n.pos + 1; i < n.end; i += 1 {
		if n.nodes[i].kind != AttrNode {
			break
		}
		c := n.nodes[i]
		if c.name.Space == "" && c.name.Local == "xmlns" {
			res[""] = c.attr
		} else if c.name.Space == "xmlns" {
			res[c.name.Local] = c.attr
		}
	}

	return res
}

// ns0: map[nstag]nsuri
func (n *Node) toXML(ns0 map[string]string) []byte {
	var res []byte
	ns := n.namespaces(ns0)

	switch n.kind {
	case StartNode:
		if n.name.Local != "" {
			res = append(res, '<')
			if nstag := findNS(ns, n.name.Space); nstag != "" {
				res = append(res, []byte(nstag)...)
				res = append(res, ':')
			}
			res = append(res, []byte(n.name.Local)...)
			for i := n.pos + 1; i < n.end; i += 1 {
				if n.nodes[i].kind != AttrNode {
					break
				}
				res = append(res, ' ')
				res = append(res, n.nodes[i].toXML(ns)...)
			}
			res = append(res, '>')
		}
		for _, c := range n.down {
			if c.kind != AttrNode {
				res = append(res, c.toXML(ns)...)
			}
		}
		if n.name.Local != "" {
			res = append(res, '<', '/')
			if nstag := findNS(ns, n.name.Space); nstag != "" {
				res = append(res, []byte(nstag)...)
				res = append(res, ':')
			}
			res = append(res, []byte(n.name.Local)...)
			res = append(res, '>')
		}
		return res
	case EndNode:
		sn := n.nodes[n.end]
		if sn.name.Local != "" {
			res = append(res, '<', '/')
			if nstag := findNS(ns, sn.name.Space); nstag != "" {
				res = append(res, []byte(nstag)...)
				res = append(res, ':')
			}
			res = append(res, []byte(sn.name.Local)...)
			res = append(res, '>')
		}
		return res
	case AttrNode:
		if nstag := findNS(ns, n.name.Space); nstag != "" {
			res = append(res, []byte(nstag)...)
			res = append(res, ':')
		}
		res = append(res, []byte(n.name.Local)...)
		res = append(res, '=', '"')
		res = appendEscaped(res, []byte(n.attr))
		res = append(res, '"')
		return res
	case TextNode:
		res = appendEscaped(res, n.text)
		return res
	case CommentNode:
		res = append(res, '<', '!', '-', '-')
		res = append(res, n.text...)
		res = append(res, '-', '-', '>')
		return res
	case ProcInstNode:
		res = append(res, '<', '?')
		res = append(res, n.text...)
		res = append(res, '?', '>')
		return res
	default:
		return nil
	}
}

func appendEscaped(result []byte, data []byte) []byte {
	for _, c := range data {
		switch c {
		case '<':
			result = append(result, []byte("&lt;")...)
			break
		case '>':
			result = append(result, []byte("&gt;")...)
			break
		case '&':
			result = append(result, []byte("&amp;")...)
			break
		case '"':
			result = append(result, []byte("&quot;")...)
			break
		case '\'':
			result = append(result, []byte("&apos;")...)
			break
		default:
			result = append(result, c)
			break
		}
	}
	return result
}

// String returns the string value of node.
//
// The string value of a node is:
//
//     - For element nodes, the concatenation of all text nodes within the element.
//     - For text nodes, the text itself.
//     - For attribute nodes, the attribute value.
//     - For comment nodes, the text within the comment delimiters.
//     - For processing instruction nodes, the content of the instruction.
//
func (node *Node) String() string {
	if node.kind == AttrNode {
		return node.attr
	}
	return string(node.Bytes())
}

func CreateTextNode(text []byte) Node {
	return *refresh([]Node{
		Node{
			kind: TextNode,
			text: text,
		},
	})
}

func (n *Node) numattributes() int {
	numattr := 0
	for i := n.pos + 1; i < n.end; i += 1 {
		if n.nodes[i].kind == AttrNode {
			numattr += 1
		} else {
			break
		}
	}
	return numattr
}

// Set the children nodes (make a copy of them)
// (and the node will become invalid, you should use n.Ref.Node instead)
func (n *Node) SetChildren(nodes ...Node) {
	switch n.kind {
	case StartNode:
		var nodelist, nodelist2 []Node
		numattr := n.numattributes()
		nodelist = append(nodelist, n.nodes[:n.pos+numattr+1]...)
		for _, nn := range nodes {
			nodelist2 = append(nodelist2, nn.extract()...)
		}
		for i := range nodelist2 {
			nodelist2[i].Ref = nil
		}
		nodelist = append(nodelist, nodelist2...)
		nodelist = append(nodelist, n.nodes[n.end:]...)
		refresh(nodelist)
		break
	default:
		panic(fmt.Sprintf("Cannot set children for node type %v", n.kind))
	}
}

// Delete current node
func (n *Node) Remove() {
	var nodelist []Node
	nodelist = append(nodelist, n.nodes[:n.pos]...)
	if n.kind == StartNode {
		nodelist = append(nodelist, n.nodes[n.end+1:]...)
	} else {
		nodelist = append(nodelist, n.nodes[n.pos+1:]...)
	}
	refresh(nodelist)
}

func (n *Node) extract() []Node {
	if n.kind == StartNode {
		return n.nodes[n.pos : n.end+1]
	} else if n.nodes != nil {
		return n.nodes[n.pos:n.end]
	} else {
		return []Node{*n}
	}
}

func (n *Node) Replace(nodes ...Node) {
	var nodelist, nodelist2 []Node
	nodelist = append(nodelist, n.nodes[:n.pos]...)
	for _, nn := range nodes {
		nodelist2 = append(nodelist2, nn.extract()...)
	}
	for i := range nodelist2 {
		nodelist2[i].Ref = nil
	}
	nodelist = append(nodelist, nodelist2...)
	if n.kind == StartNode {
		nodelist = append(nodelist, n.nodes[n.end+1:]...)
	} else {
		nodelist = append(nodelist, n.nodes[n.pos+1:]...)
	}
	refresh(nodelist)
}

func (n *Node) ReplaceInner(nodes ...Node) {
	if n.kind != StartNode {
		panic(fmt.Sprintf("ReplaceInside in %v", n.kind))
	}
	var nodelist, nodelist2 []Node
	nodelist = append(nodelist, n.nodes[:n.pos+1]...)
	for _, nn := range nodes {
		nodelist2 = append(nodelist2, nn.extract()...)
	}
	for i := range nodelist2 {
		nodelist2[i].Ref = nil
	}
	nodelist = append(nodelist, nodelist2...)
	nodelist = append(nodelist, n.nodes[n.end:]...)
	refresh(nodelist)
}

func (n *Node) InsertBefore(nodes ...Node) {
	var nodelist, nodelist2 []Node
	nodelist = append(nodelist, n.nodes[:n.pos]...)
	for _, nn := range nodes {
		nodelist2 = append(nodelist2, nn.extract()...)
	}
	for i := range nodelist2 {
		nodelist2[i].Ref = nil
	}
	nodelist = append(nodelist, nodelist2...)
	nodelist = append(nodelist, n.nodes[n.pos:]...)
	refresh(nodelist)
}

func (n *Node) InsertAfter(nodes ...Node) {
	var after int
	var nodelist, nodelist2 []Node
	if n.kind == StartNode {
		after = n.end + 1
	} else {
		after = n.pos + 1
	}
	nodelist = append(nodelist, n.nodes[:after]...)
	for _, nn := range nodes {
		nodelist2 = append(nodelist2, nn.extract()...)
	}
	for i := range nodelist2 {
		nodelist2[i].Ref = nil
	}
	nodelist = append(nodelist, nodelist2...)
	nodelist = append(nodelist, n.nodes[after:]...)
	refresh(nodelist)
}

// Set the bytes of a node
// On a start node, remove all child nodes and replace it by a single text node
// (and the node will become invalid, you should use n.Ref.Node instead)
func (n *Node) SetBytes(data []byte) {
	switch n.kind {
	case StartNode:
		var nodelist []Node
		numattr := n.numattributes()
		nodelist = append(nodelist, n.nodes[:n.pos+numattr+1]...)
		nodelist = append(nodelist, Node{
			kind:  TextNode,
			text:  data,
			nodes: n.nodes,
			up:    n,
		})
		nodelist = append(nodelist, n.nodes[n.end:]...)
		refresh(nodelist)
		break
	case AttrNode:
		n.attr = string(data)
		break
	case TextNode, CommentNode, ProcInstNode:
		n.text = data
		break
	}
}

// Set the bytes of a node
func (n *Node) SetName(local string) {
	n.SetNameNS("", local)
}

// Set the bytes of a node
func (n *Node) SetNameNS(space, local string) {
	switch n.kind {
	case StartNode, AttrNode:
		n.name.Space = space
		n.name.Local = local
		break
	case EndNode:
		sn := n.nodes[n.end]
		sn.name.Space = space
		sn.name.Local = local
		break
	}
}

// Bytes returns the string value of node as a byte slice.
// See Node.String for a description of what the string value of a node is.
func (node *Node) Bytes() []byte {
	if node.kind == AttrNode {
		return []byte(node.attr)
	}
	if node.kind != StartNode {
		return node.text
	}
	var text []byte
	for i := node.pos; i < node.end; i++ {
		if node.nodes[i].kind == TextNode {
			text = append(text, node.nodes[i].text...)
		}
	}
	return text
}

// equals returns whether the string value of node is equal to s,
// without allocating memory.
func (node *Node) equals(s string) bool {
	if node.kind == AttrNode {
		return s == node.attr
	}
	if node.kind != StartNode {
		if len(s) != len(node.text) {
			return false
		}
		for i := range s {
			if s[i] != node.text[i] {
				return false
			}
		}
		return true
	}
	si := 0
	for i := node.pos; i < node.end; i++ {
		if node.nodes[i].kind == TextNode {
			for _, c := range node.nodes[i].text {
				if si > len(s) {
					return false
				}
				if s[si] != c {
					return false
				}
				si++
			}
		}
	}
	return si == len(s)
}

// Parse reads an xml document from r, parses it, and returns its root node.
func Parse(r io.Reader) (*Node, error) {
	return ParseDecoder(xml.NewDecoder(r))
}

// ParseHTML reads an HTML-like document from r, parses it, and returns
// its root node.
func ParseHTML(r io.Reader) (*Node, error) {
	d := xml.NewDecoder(r)
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	return ParseDecoder(d)
}

// ParseDecoder parses the xml document being decoded by d and returns
// its root node.
func ParseDecoder(d *xml.Decoder) (*Node, error) {
	var nodes []Node
	var text []byte

	// The root node.
	nodes = append(nodes, Node{kind: StartNode})

	for {
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := t.(type) {
		case xml.EndElement:
			nodes = append(nodes, Node{
				kind: EndNode,
			})
		case xml.StartElement:
			nodes = append(nodes, Node{
				kind: StartNode,
				name: t.Name,
			})
			for _, attr := range t.Attr {
				nodes = append(nodes, Node{
					kind: AttrNode,
					name: attr.Name,
					attr: attr.Value,
				})
			}
		case xml.CharData:
			texti := len(text)
			text = append(text, t...)
			nodes = append(nodes, Node{
				kind: TextNode,
				text: text[texti : texti+len(t)],
			})
		case xml.Comment:
			texti := len(text)
			text = append(text, t...)
			nodes = append(nodes, Node{
				kind: CommentNode,
				text: text[texti : texti+len(t)],
			})
		case xml.ProcInst:
			texti := len(text)
			text = append(text, t.Inst...)
			nodes = append(nodes, Node{
				kind: ProcInstNode,
				name: xml.Name{Local: t.Target},
				text: text[texti : texti+len(t.Inst)],
			})
		}
	}

	// Close the root node.
	nodes = append(nodes, Node{kind: EndNode})

	node := refresh(nodes)

	if node == nil {
		return nil, io.EOF
	} else {
		return node, nil
	}
}

// Refresh all nodes in relation to each other
// Return the root node (or nil if there is a problem)
func refresh(nodes []Node) *Node {
	stack := make([]*Node, 0, len(nodes))
	downs := make([]*Node, len(nodes))
	downCount := 0

	for pos := range nodes {

		nodes[pos].nodes = nodes
		nodes[pos].pos = pos
		nodes[pos].end = pos + 1
		if nodes[pos].Ref == nil {
			nodes[pos].Ref = &NodeRef{nil}
		}
		nodes[pos].Ref.Node = &nodes[pos]

		switch nodes[pos].kind {

		case StartNode, AttrNode, TextNode, CommentNode, ProcInstNode:
			node := &nodes[pos]
			if len(stack) > 0 {
				node.up = stack[len(stack)-1]
			}
			if node.kind == StartNode {
				stack = append(stack, node)
			}
			if len(stack) == 0 {
				return node
			}

		case EndNode:
			node := stack[len(stack)-1]
			node.end = pos
			nodes[pos].end = node.pos
			stack = stack[:len(stack)-1]

			// Compute downs. Doing that here is what enables the
			// use of a slice of a contiguous pre-allocated block.
			node.down = downs[downCount:downCount]
			for i := node.pos + 1; i < pos; i++ {
				if nodes[i].up == node {
					switch nodes[i].kind {
					case StartNode, TextNode, CommentNode, ProcInstNode:
						node.down = append(node.down, &nodes[i])
						downCount++
					}
				}
			}
			if len(stack) == 0 {
				return node
			}
		}
	}
	return nil
}

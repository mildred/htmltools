html-template
=============

html-template will parse template tags in HTML/XML streams, interpret them and
instanciate them. Templates are composed of:

- a data source: a DOM tree
- a template tag `<template/>` containing the markup
- a template instance tag `<template-instance/>` that contains directives to map
  the data source to the markup. When evaluated, it will be replaced by the
  resulting markup.

`<template-instance/>`
----------------------

Attributes:

- `src`:   document to use as data source, empty for the current document
- `using`: id of the `<template/>` tag to use for markup
- `if`:    xpath of an element must be found for the template to evaluate

The `<template-instance/>` tag must contains directives on which element in the
source document to map to which tag in the template markup. Directives allowed
are `<map/>` tags.

If `using` is not specified, the `<template/>` tag can be included as a child of
the `<template-instance/>` tag.

`<map/>`
--------

A map directive describe mapping from the data source to the template markup.
Its most common form is:

    <map from="<xpath in source>" to="<xpath in template>" />

This will put the content found by following the from expression in the element
found using the to expression. There are multiple attributes that can be used to
customize its working:

- from:     xpath to locate the in the source document
- to:       xpath to locate the target template node
- data:     replaces the `from` attribute. Take a build-in source
- format:   format filter to apply to the data before it is applied
- only-if:  When `only-if="empty"`, the mapping will only be performed if the
            node pointed by the `to` xpath expression is empty. It is useful to
            perform multiple mapping on the same target node but stop when a
            mapping is successful
- multiple: perform an iteration on the data source, duplicating the template
            markup
- fetch:    Changes the source document

### Built-in sources (data attribute) ###

- relative-url: the relative URL to the current source file
- relative-dir: the relative link to the directory containing the current source
                file

### Format filters ###

- text:          convert to text
- split:         convert to text then split on spaces and return a list of text nodes
- link-relative: consider the source text is a link relative to the source
                 document. Transform the link to make it relative to the
                 document beeing processed. Useful when the data source is not
                 the document being templated.
- datetime:      parse the date in the source text and format it according to
                 the additional `strftime` attribute

### `<map fetch="resource"/>` ###

There is a form of the map directive where two attributes are present only:
`from` and `fetch="resource"`. This form allow to have children which are parsed
in the new context.

The from attributes selects an url in the source document. This URL must be
relative to the source document itself. It will then fetch this URL and replace
the current source document by this new document.

Allowed children: `<map/>`

### `<map multiple="true"/>` ###

When the `from` expression finds multiple elements, it will put them all inside
one element pointed by `to` except when `multiple="true"` is specified. In that
case, the element pointed by the `to` expression is duplicated the number of
times necessary to fit all elements matched in the `from` expression.

Instead of directly mapping each source elements to each destination element,
sub-directives can be specified to correctly map each part of the source to a
matching part of the destination. For each child directive, the current XPath
node is set to either the matched node by `from` or the duplicated node matched
by `to`.

Allowed children:

- `<map/>`:  to perform the mapping
- `<sort/>`: to define the order the elements must appear in

`<sort/>`
---------

Sorting elements must appear within a `<map multiple="true"/>` element. They
define the order the elements must appear in the templated result. A sort
element have two forms:

- `<sort asc="<xpath>"/>`
- `<sort desc="<xpath>"/>`


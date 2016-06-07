package htmldepth

import (
	"fmt"
	"strings"
)

type HTMLDepth struct {
	Breadcrumb []string
}

func (d *HTMLDepth) Start(name string) {
	d.Breadcrumb = append(d.Breadcrumb, name)
}

func (d *HTMLDepth) Depth() int {
	return len(d.Breadcrumb)
}

func (d *HTMLDepth) Stop(name string) error {
	path := strings.Join(d.Breadcrumb, "/")
	for name != "" && len(d.Breadcrumb) > 0 && d.Breadcrumb[len(d.Breadcrumb)-1] != name {
		d.Breadcrumb = d.Breadcrumb[:len(d.Breadcrumb)-1]
	}
	if len(d.Breadcrumb) == 0 {
		return fmt.Errorf("%s: Non matching close tag </%s>", path, name)
	}
	path = strings.Join(d.Breadcrumb, "/")
	if name != "" && d.Breadcrumb[len(d.Breadcrumb)-1] != name {
		return fmt.Errorf("%s: Non matching close tag </%s>", path, name)
	}
	d.Breadcrumb = d.Breadcrumb[:len(d.Breadcrumb)-1]
	return nil
}

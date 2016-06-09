package relurl

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var (
	_ = os.Stderr
	_ = fmt.Fprintf
)

func UrlJoinString(uBase, u, cwd string) (string, error) {
	ub, err := url.Parse(uBase)
	if err != nil {
		return "", err
	}
	uu, err := url.Parse(u)
	if err != nil {
		return "", err
	}
	return UrlJoin(ub, uu, cwd)
}

// Join two URL
// cwd is used in case the two URL are paths. It is the directory to which the
// given base URL is relative to. It should be absolute.
func UrlJoin(uBase, u *url.URL, cwd string) (string, error) {
	if u.Scheme != "" {
		return u.String(), nil
	}
	u.Scheme = uBase.Scheme
	if uBase.Opaque != "" {
		u.Opaque = uBase.Opaque
		return u.String(), nil
	}
	if u.Host != "" {
		return u.String(), nil
	}
	u.User = uBase.User
	u.Host = uBase.Host
	if u.Path != "" && strings.HasPrefix(u.Path, "/") {
		return u.String(), nil
	}
	if u.Path != "" {
		uPath := u.Path
		u.Path = filepath.Join(uBase.Path, u.Path)
		if u.Scheme == "" && u.User == nil && u.Host == "" && cwd != "" {
			// Pure path
			base := uBase.Path
			if !strings.HasPrefix(base, "/") {
				//fmt.Fprintf(os.Stderr, "base = Join(%#v, %#v)) = %#v\n", cwd, base, filepath.Join(cwd, base))
				base = filepath.Join(cwd, base)
			}
			rel, err := filepath.Rel(cwd, filepath.Join(base, uPath))
			//fmt.Fprintf(os.Stderr, "Rel(%#v, Join(%#v, %#v)) = %#v\n", cwd, base, uPath, rel)
			if err != nil {
				return "", err
			}
			u.Path = rel
		}
	}
	if u.RawQuery != "" {
		return u.String(), nil
	}
	u.RawQuery = uBase.RawQuery
	if u.Fragment != "" {
		return u.String(), nil
	}
	u.Fragment = uBase.Fragment
	return u.String(), nil
}

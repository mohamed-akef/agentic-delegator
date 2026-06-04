// Package static serves the compiled CSS (and any future static assets) via
// an embed.FS. The compiled CSS at css/app.css is produced by `make css`
// from web/input.css; commit the regenerated file alongside any templ change
// that introduces new Tailwind classes.
package static

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed css/app.css
var files embed.FS

// FS returns the embedded filesystem rooted at this package (so files live
// at paths like "css/app.css").
func FS() fs.FS { return files }

// Handler returns an http.Handler that serves files under /static/* — strip
// the prefix before mounting.
func Handler() http.Handler {
	return http.FileServer(http.FS(files))
}

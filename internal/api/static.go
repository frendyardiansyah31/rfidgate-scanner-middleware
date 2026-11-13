package api

import (
	"io/fs"
	"net/http"
	"os"
)

// staticHandler serves the dashboard UI. If webDir is set the files are read
// from disk (edit and refresh); otherwise the embedded copy is served.
func staticHandler(webDir string, embedded fs.FS) http.Handler {
	root := embedded
	if webDir != "" {
		root = os.DirFS(webDir)
	}
	files := http.FileServerFS(root)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always revalidate so a new build or an edited file shows up on refresh.
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})
}

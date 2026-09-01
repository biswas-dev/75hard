package api

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// SPAHandler serves the built React app from dir, falling back to index.html
// for any path that isn't a real file so client-side routing works on a hard
// refresh of a deep link.
//
// When dir is empty or missing, it returns a handler explaining that the
// frontend wasn't built — useful when running the API alone against the Vite
// dev server.
func SPAHandler(dir string) http.Handler {
	if dir == "" {
		return placeholderSPA("frontend not bundled; run the Vite dev server")
	}
	info, err := os.Stat(path.Join(dir, "index.html"))
	if err != nil || info.IsDir() {
		return placeholderSPA("frontend build not found at " + dir)
	}

	root := os.DirFS(dir)
	fileServer := http.FileServer(http.FS(root))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if upath == "" {
			upath = "index.html"
		}

		if f, err := fs.Stat(root, upath); err == nil && !f.IsDir() {
			// Vite emits content-hashed asset names, so those are safe to
			// cache forever. index.html must not be.
			if strings.HasPrefix(upath, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, root, "index.html")
	})
}

func placeholderSPA(message string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("75hard: " + message + "\n"))
	})
}

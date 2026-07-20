package web

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

func spaHandler(assets fs.FS) http.HandlerFunc {
	files := http.FileServer(http.FS(assets))
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "." {
			name = "index.html"
		}
		if info, err := fs.Stat(assets, name); err != nil || info.IsDir() {
			// 带扩展名的路径通常是静态资源，缺失时必须返回 404；只有无扩展名
			// 的前端路由才回退到 index.html，避免把 HTML 当作 JS/CSS 返回。
			if path.Ext(name) != "" {
				http.NotFound(w, r)
				return
			}
			clone := r.Clone(r.Context())
			clone.URL.Path = "/"
			files.ServeHTTP(w, clone)
			return
		}
		files.ServeHTTP(w, r)
	}
}

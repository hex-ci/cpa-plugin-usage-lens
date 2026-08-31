// resources.go: embeds the built panel (Vite dist) and registers every asset
// as a resource route + a /panel menu entry.
package main

import (
	"embed"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

//go:embed all:panel/dist
var panelFS embed.FS

// panelAssets enumerates the embedded dist directory, returning one resource
// route per file under /v0/resource/plugins/<id>/.
func panelAssets() []pluginapi.ResourceRoute {
	var routes []pluginapi.ResourceRoute
	_ = fs.WalkDir(panelFS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		path := "/" + strings.TrimPrefix(p, "panel/dist/")
		if path == "/index.html" {
			// 入口资源带 Menu，CPA 主面板左侧菜单据此渲染入口项。
			routes = append(routes, pluginapi.ResourceRoute{
				Path:        "/panel",
				Menu:        "Usage Lens",
				Description: "用量分析面板",
			})
			return nil
		}
		routes = append(routes, pluginapi.ResourceRoute{
			Path:        path,
			Description: "usage-lens panel asset",
		})
		return nil
	})
	return routes
}

// serveResource resolves a resource path against the embedded dist and returns
// the file with a mime type and sensible caching (immutable for hashed assets).
func serveResource(sub string) pluginapi.ManagementResponse {
	sub = strings.Trim(strings.TrimPrefix(sub, "/"), "/")
	if sub == "" || sub == "panel" {
		sub = "index.html"
	}
	data, err := panelFS.ReadFile("panel/dist/" + sub)
	if err != nil {
		return mgmtJSON(http.StatusNotFound, map[string]any{"error": "not found"})
	}
	return mgmtAsset(http.StatusOK, data, sub)
}

func mgmtAsset(status int, body []byte, path string) pluginapi.ManagementResponse {
	h := http.Header{}
	h.Set("Content-Type", mimeFor(path))
	ext := strings.ToLower(filepath.Ext(path))
	// Hashed assets (Vite emits e.g. assets/index-<hash>.js) are immutable.
	if strings.Contains(filepath.Base(path), "-") && (ext == ".js" || ext == ".css") {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "no-cache")
	}
	return pluginapi.ManagementResponse{StatusCode: status, Headers: h, Body: body}
}

func mimeFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "application/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ico":
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}

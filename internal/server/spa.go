package server

import (
	"io/fs"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/Gouryella/supabase-studio-go/internal/config"
)

func spaHandler(static fs.FS, cfg config.Config) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(static))
	dynamicRoutes := buildDynamicRoutes(static)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		requestPath := r.URL.Path
		if cfg.BasePath != "" {
			base := strings.TrimSuffix(cfg.BasePath, "/")
			if strings.HasPrefix(requestPath, base) {
				requestPath = strings.TrimPrefix(requestPath, base)
				if requestPath == "" {
					requestPath = "/"
				}
			}
		}

		candidate, isHTML := resolveStaticPath(static, requestPath, dynamicRoutes)
		if candidate != "" {
			if strings.HasSuffix(requestPath, ".ts") {
				w.Header().Set("Content-Type", "text/typescript")
			}
			if isHTML {
				w.Header().Set("Cache-Control", "no-cache")
			} else {
				w.Header().Set("Cache-Control", cacheControlForPath("/"+candidate))
			}
			r.URL.Path = "/" + candidate
			fileServer.ServeHTTP(w, r)
			return
		}

		if fileExists(static, "404.html") {
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusNotFound)
			r.URL.Path = "/404.html"
			fileServer.ServeHTTP(w, r)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

func fileExists(fsys fs.FS, name string) bool {
	if name == "" {
		return false
	}
	file, err := fsys.Open(name)
	if err != nil {
		return false
	}
	_ = file.Close()
	return true
}

type routeSegment struct {
	value    string
	dynamic  bool
	catchAll bool
	optional bool
}

type dynamicRoute struct {
	segments     []routeSegment
	filePath     string
	segmentCount int
	staticCount  int
	catchAlls    int
}

func buildDynamicRoutes(static fs.FS) []dynamicRoute {
	var routes []dynamicRoute
	_ = fs.WalkDir(static, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".html") {
			return nil
		}
		if !strings.Contains(path, "[") {
			return nil
		}

		routePath := strings.TrimSuffix(path, ".html")
		routePath = strings.TrimSuffix(routePath, "/index")
		routePath = strings.TrimPrefix(routePath, "./")
		routePath = strings.TrimPrefix(routePath, "/")
		if routePath == "" {
			return nil
		}

		segments := strings.Split(routePath, "/")
		parsed := make([]routeSegment, 0, len(segments))
		staticCount := 0
		catchAlls := 0
		for _, seg := range segments {
			seg = strings.TrimSpace(seg)
			routeSeg := routeSegment{value: seg}
			if strings.HasPrefix(seg, "[[...") && strings.HasSuffix(seg, "]]") {
				routeSeg.dynamic = true
				routeSeg.catchAll = true
				routeSeg.optional = true
				catchAlls++
			} else if strings.HasPrefix(seg, "[...") && strings.HasSuffix(seg, "]") {
				routeSeg.dynamic = true
				routeSeg.catchAll = true
				catchAlls++
			} else if strings.HasPrefix(seg, "[") && strings.HasSuffix(seg, "]") {
				routeSeg.dynamic = true
			} else {
				staticCount++
			}
			parsed = append(parsed, routeSeg)
		}

		routes = append(routes, dynamicRoute{
			segments:     parsed,
			filePath:     path,
			segmentCount: len(parsed),
			staticCount:  staticCount,
			catchAlls:    catchAlls,
		})
		return nil
	})

	sort.SliceStable(routes, func(i, j int) bool {
		if routes[i].segmentCount != routes[j].segmentCount {
			return routes[i].segmentCount > routes[j].segmentCount
		}
		if routes[i].staticCount != routes[j].staticCount {
			return routes[i].staticCount > routes[j].staticCount
		}
		return routes[i].catchAlls < routes[j].catchAlls
	})

	return routes
}

func resolveStaticPath(static fs.FS, requestPath string, dynamicRoutes []dynamicRoute) (string, bool) {
	trimmed := strings.TrimPrefix(path.Clean(requestPath), "/")
	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "." {
		trimmed = ""
	}

	if trimmed == "" {
		if fileExists(static, "index.html") {
			return "index.html", true
		}
	} else if info, err := fs.Stat(static, trimmed); err == nil {
		if info.IsDir() {
			indexPath := path.Join(trimmed, "index.html")
			if fileExists(static, indexPath) {
				return indexPath, true
			}
		} else {
			return trimmed, strings.HasSuffix(trimmed, ".html")
		}
	}

	if trimmed != "" && !strings.Contains(path.Base(trimmed), ".") {
		htmlPath := trimmed + ".html"
		if fileExists(static, htmlPath) {
			return htmlPath, true
		}
	}

	if trimmed != "" {
		if match := matchDynamicRoute(trimmed, dynamicRoutes); match != "" {
			return match, true
		}
	}

	return "", false
}

func matchDynamicRoute(requestPath string, routes []dynamicRoute) string {
	segments := strings.Split(strings.Trim(requestPath, "/"), "/")
	if len(segments) == 1 && segments[0] == "" {
		segments = []string{}
	}

	for _, route := range routes {
		if matchesSegments(segments, route.segments) {
			return route.filePath
		}
	}
	return ""
}

func matchesSegments(request []string, pattern []routeSegment) bool {
	if len(pattern) == 0 {
		return len(request) == 0
	}
	for i, seg := range pattern {
		if seg.catchAll {
			if !seg.optional && i >= len(request) {
				return false
			}
			return true
		}
		if i >= len(request) {
			return false
		}
		if !seg.dynamic && seg.value != request[i] {
			return false
		}
	}
	return len(request) == len(pattern)
}

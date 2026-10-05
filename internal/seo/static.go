package seo

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// StaticPaths are the pages whose content does not depend on the database.
// They can be written to disk once and served by any web server or CDN; every
// other route (records, models, sites, reports) is dynamic and stays with the
// Go server.
var StaticPaths = []string{"/", "/baselines", "/get-badge", "/method", "/integrate"}

// StaticFile is one generated file, Path relative to the site root.
type StaticFile struct {
	Path string
	Data []byte
}

// StaticFiles renders the static pages into the given index.html template.
func StaticFiles(index []byte, site string) ([]StaticFile, error) {
	var out []StaticFile
	for _, path := range StaticPaths {
		page, ok := Lookup(path)
		if !ok {
			return nil, fmt.Errorf("static path %s is not a known page", path)
		}
		name := "index.html"
		if path != "/" {
			name = strings.TrimPrefix(path, "/") + "/index.html"
		}
		out = append(out, StaticFile{name, Render(index, site, page)})
	}
	out = append(out,
		StaticFile{"404.html", Render(index, site, notFoundPage)},
		StaticFile{"robots.txt", Robots(site)},
	)
	return out, nil
}

// WriteStatic copies the built frontend (distDir) to outDir and writes the
// generated pages over it, so outDir is a complete deployable tree for the
// static pages. outDir is replaced file by file; it is never emptied.
func WriteStatic(distDir, outDir, site string) ([]string, error) {
	index, err := os.ReadFile(filepath.Join(distDir, "index.html"))
	if err != nil {
		return nil, err
	}
	files, err := StaticFiles(index, site)
	if err != nil {
		return nil, err
	}
	err = filepath.WalkDir(distDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(distDir, p)
		if rel == "." || rel == "index.html" {
			return nil
		}
		dst := filepath.Join(outDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		return nil, err
	}
	var written []string
	for _, f := range files {
		dst := filepath.Join(outDir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, f.Data, 0o644); err != nil {
			return nil, err
		}
		written = append(written, f.Path)
	}
	return written, nil
}

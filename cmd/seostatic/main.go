// Command seostatic writes the database-independent pages (home, baselines,
// get-badge, method, 404, robots.txt) as static HTML next to the
// built frontend, ready for a CDN or nginx. Dynamic routes stay with the Go
// server; see docs/static-deploy.md.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"model-check/internal/seo"
)

func main() {
	site := flag.String("site", os.Getenv("MODEL_CHECK_SITE_URL"), "public origin, e.g. https://modelsexam.com")
	dist := flag.String("dist", "web/dist", "built frontend directory")
	out := flag.String("out", "web/dist-static", "output directory")
	flag.Parse()
	u, err := url.Parse(strings.TrimSpace(*site))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || (u.Path != "" && u.Path != "/") {
		log.Fatal("-site (or MODEL_CHECK_SITE_URL) must be an origin such as https://modelsexam.com")
	}
	written, err := seo.WriteStatic(*dist, *out, u.Scheme+"://"+u.Host)
	if err != nil {
		log.Fatal(err)
	}
	for _, p := range written {
		fmt.Println(*out + "/" + p)
	}
}

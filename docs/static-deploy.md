# Static pages and deployment

The Go server already writes real HTML (head tags, JSON-LD, plain body) for
every route, so a crawler never needs JavaScript. For the pages that do not
depend on the database there is also a static build, useful behind a CDN or
nginx.

```sh
export MODEL_CHECK_SITE_URL=https://your-domain
make static        # builds web/dist, then writes web/dist-static
```

`web/dist-static` is a complete tree: the built assets plus
`index.html`, `baselines/`, `get-badge/`, `method/`, `404.html`
and `robots.txt`, each with its own title, description, canonical, hreflang,
Open Graph and structured data. It is not embedded in the Go binary.

Static: `/`, `/baselines`, `/get-badge`, `/method`.
Dynamic (keep on the Go server): `/records`, `/models`, `/models/<name>`,
`/sites/<host>`, `/reports/<id>`, `/sitemap.xml`, `/robots.txt` (the Go one lists
reports), `/api/`, `/badge/`, `/embed/`.

The static home page has no live stats or boards; the Go server fills those.
Use static serving only if you want the shell cached at the edge.

nginx example:

```nginx
root /srv/modelsexam/dist-static;
location ~ ^/(records|models|sites|reports|api|badge|embed|sitemap\.xml|robots\.txt) {
    proxy_pass http://127.0.0.1:8080;
}
location / { try_files $uri $uri/ =404; }
error_page 404 /404.html;
gzip on; gzip_types text/css application/javascript application/json image/svg+xml;
```

(Without nginx, the Go server compresses text responses itself.)

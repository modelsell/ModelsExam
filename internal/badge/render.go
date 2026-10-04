package badge

import (
	"encoding/json"
	"fmt"
	"html"
)

// face is what the right half of a badge says: a verdict, an optional score
// and the colour key that picks the accent.
type face struct {
	text  string
	score string
	key   string // conformant | mostly | review | muted
}

func faceOf(r Result) face {
	switch {
	case r.Status == "stale":
		return face{"Check expired", "", "muted"}
	case r.Status != "ok" || r.Score == nil:
		return face{"Not checked yet", "", "muted"}
	case r.Tier == "conformant":
		return face{"Conformant", fmt.Sprint(*r.Score), "conformant"}
	case r.Tier == "mostly":
		return face{"Mostly conformant", fmt.Sprint(*r.Score), "mostly"}
	default:
		return face{"Review suggested", fmt.Sprint(*r.Score), "review"}
	}
}

// palette is one theme. Accent text sits on a tint of the same hue; every
// pairing keeps at least 4.5:1 contrast.
type palette struct {
	bg, border, fg, soft string
	accent               map[string][2]string // key -> {text, tint}
}

var themes = map[string]palette{
	"light": {"#ffffff", "#d3dae6", "#0e1b2e", "#5b6779", map[string][2]string{
		"conformant": {"#116532", "#e4f4ea"}, "mostly": {"#8a4f00", "#fbefd6"},
		"review": {"#a9211a", "#fcebe9"}, "muted": {"#505b6b", "#eceff4"}}},
	"dark": {"#0a0e16", "#2a354d", "#e8edf5", "#93a0b5", map[string][2]string{
		"conformant": {"#4ade80", "#10301e"}, "mostly": {"#fbbf24", "#33270a"},
		"review": {"#ff8a84", "#3a1416"}, "muted": {"#a9b4c6", "#1c2330"}}},
}

// ThemeOf maps a query value to light, dark or auto (follows the viewer's
// colour scheme). Anything else is auto.
func ThemeOf(s string) string {
	if s == "light" || s == "dark" {
		return s
	}
	return "auto"
}

func textWidth(s string, ascii, wide int) int {
	w := 0
	for _, r := range s {
		if r > 0x2e80 {
			w += wide
		} else {
			w += ascii
		}
	}
	return w
}

// SVG renders the badge: a plate with the ModelsExam mark on the left and the
// verdict on the tinted right half. It stands alone, so it works wherever an
// <img> does. theme is light, dark or auto.
func SVG(r Result, theme string) []byte { return svg(r, faceOf(r), theme) }

// MismatchSVG is served when the image is requested by a page that is not the
// badge's domain.
func MismatchSVG(domain, theme string) []byte {
	return svg(Result{Domain: domain}, face{"Not issued for this site", "", "muted"}, theme)
}

func cssVars(p palette, key string) string {
	a := p.accent[key]
	return fmt.Sprintf("--bg:%s;--bd:%s;--fg:%s;--sf:%s;--ac:%s;--tn:%s", p.bg, p.border, p.fg, p.soft, a[0], a[1])
}

func svg(r Result, f face, theme string) []byte {
	theme = ThemeOf(theme)
	const h = 28
	// Left: the shield mark alone. Right: verdict, then score.
	const wl = 28
	wv := textWidth(f.text, 8, 12)
	wr := 12 + wv + 12
	scoreX := 0
	if f.score != "" {
		scoreX = wr
		wr += textWidth(f.score, 8, 8) + 12
	}
	total := wl + wr
	title := "ModelsExam: " + f.text
	if f.score != "" {
		title += " " + f.score + "/100"
	}
	if r.Domain != "" {
		title += " (" + r.Domain + ")"
	}
	light := themes["light"]
	dark := themes["dark"]
	var css string
	switch theme {
	case "dark":
		css = ":root{" + cssVars(dark, f.key) + "}"
	case "light":
		css = ":root{" + cssVars(light, f.key) + "}"
	default:
		css = ":root{" + cssVars(light, f.key) + "}@media (prefers-color-scheme:dark){:root{" + cssVars(dark, f.key) + "}}"
	}
	score := ""
	if f.score != "" {
		score = fmt.Sprintf(`<text class="n" x="%d" y="18">%s</text>`, wl+scoreX, html.EscapeString(f.score))
	}
	out := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%[1]d" height="%[2]d" viewBox="0 0 %[1]d %[2]d" role="img" aria-label="%[3]s">`+
		`<title>%[3]s</title>`+
		`<style>%[4]s .t{font:600 12px Verdana,DejaVu Sans,sans-serif}.n{font:700 12px ui-monospace,Menlo,Consolas,monospace;fill:var(--ac)}</style>`+
		`<clipPath id="c"><rect width="%[1]d" height="%[2]d" rx="7"/></clipPath>`+
		`<g clip-path="url(#c)"><rect width="%[1]d" height="%[2]d" fill="var(--bg)"/>`+
		`<rect x="%[5]d" width="%[6]d" height="%[2]d" fill="var(--tn)"/>`+
		`<rect x="%[5]d" width="1" height="%[2]d" fill="var(--bd)"/></g>`+
		`<rect x=".5" y=".5" width="%[7]d" height="%[8]d" rx="6.5" fill="none" stroke="var(--bd)"/>`+
		`<path d="M20 7.4 14.6 9.3v4.2c0 3.2 2.2 5.7 5.4 7.5 3.2-1.8 5.4-4.3 5.4-7.5V9.3L20 7.4Z" transform="translate(-6 -1.6)" fill="none" stroke="var(--fg)" stroke-width="1.3" stroke-linejoin="round"/>`+
		`<path d="m11.5 14.3 1.8 1.8 3.3-3.6" fill="none" stroke="var(--ac)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" transform="translate(0 -.4)"/>`+
		`<text class="t" x="%[9]d" y="18" fill="var(--ac)">%[10]s</text>%[11]s</svg>`,
		total, h, html.EscapeString(title), css, wl, wr, total-1, h-1, wl+12, html.EscapeString(f.text), score)
	return []byte(out)
}

// Script is the embeddable widget for one domain. It reads the badge from the
// ModelsExam API and draws it in a shadow root, so the host page's CSS cannot
// break it and it cannot touch the host page. apiBase is the public origin of
// this service. The theme comes from data-theme on the script tag (or ?theme=
// on its URL): light, dark, or auto (the default).
func Script(domain, apiBase string) []byte {
	d, _ := json.Marshal(domain)
	a, _ := json.Marshal(apiBase)
	return []byte("/* ModelsExam badge for " + html.EscapeString(domain) + " */\n(function () {\n  var DOMAIN = " + string(d) + ", API = " + string(a) + ";" + scriptBody)
}

const scriptBody = `
  if (window.__modelsexamBadge === DOMAIN) return;
  window.__modelsexamBadge = DOMAIN;
  var ME = document.currentScript;
  var zh = /^zh/i.test(navigator.language || "");
  var T = zh
    ? { conformant: "符合", mostly: "基本符合", review: "建议核对", stale: "检测已过期", checked: "检测时间" }
    : { conformant: "Conformant", mostly: "Mostly conformant", review: "Review suggested", stale: "Check expired", checked: "Checked" };
  var PAL = {
    light: { bg: "#ffffff", bd: "#d3dae6", fg: "#0e1b2e",
      conformant: ["#116532", "#e4f4ea"], mostly: ["#8a4f00", "#fbefd6"], review: ["#a9211a", "#fcebe9"], stale: ["#505b6b", "#eceff4"] },
    dark: { bg: "#0a0e16", bd: "#2a354d", fg: "#e8edf5",
      conformant: ["#4ade80", "#10301e"], mostly: ["#fbbf24", "#33270a"], review: ["#ff8a84", "#3a1416"], stale: ["#a9b4c6", "#1c2330"] }
  };
  function pref() {
    var v = "";
    try {
      v = (ME && ME.getAttribute("data-theme")) || (ME && ME.src && new URL(ME.src).searchParams.get("theme")) || "";
    } catch (e) {}
    return v === "light" || v === "dark" ? v : "auto";
  }
  function isDark() {
    var d = document.documentElement, b = document.body;
    var hint = [d.className, b && b.className, d.getAttribute("data-theme"), d.style.colorScheme].join(" ");
    if (/\bdark\b/i.test(hint)) return true;
    if (/\blight\b/i.test(hint)) return false;
    return !!(window.matchMedia && matchMedia("(prefers-color-scheme: dark)").matches);
  }
  var SHIELD = '<svg viewBox="0 0 16 16" width="15" height="15" aria-hidden="true"><path d="M8 1.2 2.6 3.1v4.2c0 3.2 2.2 5.7 5.4 7.5 3.2-1.8 5.4-4.3 5.4-7.5V3.1L8 1.2Z" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round"/><path class="k" d="m5.5 8.1 1.8 1.8 3.3-3.6" fill="none" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>';
  var CSS = ":host{all:initial}a{display:inline-flex;align-items:stretch;border-radius:8px;overflow:hidden;text-decoration:none;" +
    "font:600 12px/1 system-ui,-apple-system,Segoe UI,sans-serif;border:1px solid var(--bd);background:var(--bg);color:var(--fg);box-shadow:0 1px 2px rgba(0,0,0,.12)}" +
    "span{display:flex;align-items:center;white-space:nowrap}.l{padding:7px 9px}" +
    ".r{gap:9px;padding:7px 12px;background:var(--tn);color:var(--ac);border-left:1px solid var(--bd)}" +
    ".s{font:700 12px ui-monospace,Menlo,Consolas,monospace}.k{stroke:var(--ac)}a:focus-visible{outline:2px solid var(--ac);outline-offset:2px}";
  function draw(root, data) {
    var key = data.status === "stale" ? "stale" : data.tier;
    if (!T[key]) return false;
    var p = pref(), pal = PAL[p === "auto" ? (isDark() ? "dark" : "light") : p], ac = pal[key];
    var a = document.createElement("a");
    a.href = API + "/sites/" + encodeURIComponent(DOMAIN);
    a.target = "_blank";
    a.rel = "noopener";
    a.style.cssText = "--bg:" + pal.bg + ";--bd:" + pal.bd + ";--fg:" + pal.fg + ";--ac:" + ac[0] + ";--tn:" + ac[1];
    var when = data.checked_at ? new Date(data.checked_at).toLocaleDateString() : "";
    a.title = "ModelsExam: " + DOMAIN + (when ? " (" + T.checked + " " + when + ")" : "");
    var l = document.createElement("span");
    l.className = "l";
    l.innerHTML = SHIELD;
    var r = document.createElement("span");
    r.className = "r";
    r.textContent = T[key];
    if (key !== "stale" && data.score != null) {
      var s = document.createElement("span");
      s.className = "s";
      s.textContent = data.score;
      r.appendChild(s);
    }
    a.appendChild(l);
    a.appendChild(r);
    var style = document.createElement("style");
    style.textContent = CSS;
    root.appendChild(style);
    root.appendChild(a);
    return true;
  }
  function mount() {
    var slot = document.getElementById("modelsexam-badge");
    var host = document.createElement("div");
    if (slot) slot.appendChild(host);
    else {
      host.style.cssText = "position:fixed;right:16px;bottom:16px;z-index:2147483000";
      document.body.appendChild(host);
    }
    var root = host.attachShadow ? host.attachShadow({ mode: "open" }) : host;
    fetch(API + "/api/badge/" + encodeURIComponent(DOMAIN), { credentials: "omit" })
      .then(function (res) { return res.ok ? res.json() : null; })
      .then(function (json) {
        if (!json || !json.success || !draw(root, json.data)) host.remove();
      })
      .catch(function () { host.remove(); });
  }
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", mount);
  else mount();
})();
`

// Package webui renders the shared, script-free contributor and operations UI.
package webui

import (
	"bytes"
	"html/template"
	"net/http"
)

// Page carries trusted template selection and escaped request-specific data.
type Page struct {
	Title, Intro, Area, Path, CSRF string
	Data                           any
}

// Render escapes all data through html/template and buffers output so template
// failures cannot leave a successful, partly rendered response.
func Render(w http.ResponseWriter, status int, p Page, body string) {
	t, err := template.New("page").Funcs(template.FuncMap{"portalNav": func() []navItem { return portalNav }, "adminNav": func() []navItem { return adminNav }}).Parse(shell + `{{define "body"}}` + body + `{{end}}`)
	if err != nil {
		http.Error(w, "This page could not be loaded. Please try again.", http.StatusInternalServerError)
		return
	}
	var b bytes.Buffer
	if err = t.ExecuteTemplate(&b, "page", p); err != nil {
		http.Error(w, "This page could not be loaded. Please try again.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.WriteHeader(status)
	_, _ = w.Write(b.Bytes())
}

type navItem struct{ URL, Label string }

var portalNav = []navItem{{"/portal/", "Overview"}, {"/portal/apply", "Roles & applications"}, {"/portal/submissions", "My submissions"}, {"/portal/challenge", "Weekly challenge"}}
var adminNav = []navItem{{"/admin/", "Overview"}, {"/admin/portal/applications", "Roles & applications"}, {"/admin/portal/submissions", "Submission review"}, {"/admin/portal/challenge", "Weekly challenge"}, {"/admin/portal/terms", "Contribution terms"}, {"/admin/text", "Text releases"}, {"/admin/user-terms", "User terms"}, {"/admin/notices", "System notices"}, {"/admin/reports", "Reports"}, {"/admin/feedback", "Feedback"}, {"/admin/economy", "Accounts & economy"}}

const shell = `{{define "page"}}<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="color-scheme" content="light"><title>{{.Title}} · Knowoff</title><style>
:root{--ink:#191919;--paper:#f7f3eb;--purple:#b69aef;--mint:#bce7c0;--pink:#ffa4cb;--line:2px solid var(--ink)}*{box-sizing:border-box}body{margin:0;color:var(--ink);background:var(--paper);font:1rem/1.55 ui-sans-serif,system-ui,sans-serif}a{color:inherit;text-underline-offset:4px}::selection{background:var(--pink);color:var(--ink)}:focus-visible{outline:3px solid #654298;outline-offset:4px}input,textarea{caret-color:#654298}header{padding:18px 28px;border-bottom:var(--line);display:flex;align-items:center;justify-content:space-between;gap:16px;background:var(--purple)}.brand{font-size:1.65rem;font-weight:900;letter-spacing:-.03em;text-decoration:none}.brand span{font-size:1rem;font-weight:650;letter-spacing:0;margin-left:12px}header form{margin:0}.layout{display:grid;grid-template-columns:215px minmax(0,1fr);max-width:1600px;margin:0 auto}.layout.solo{display:block;max-width:920px}nav{padding:24px 16px;border-right:var(--line)}nav a{display:block;padding:10px 12px;min-height:48px;margin-bottom:6px;border:2px solid transparent;border-radius:8px;text-decoration:none;font-weight:650}nav a[aria-current=page]{border-color:var(--ink);background:var(--mint);box-shadow:3px 3px 0 var(--ink)}nav a:hover{background:#e6deef}main{min-width:0;padding:30px 36px 70px}h1{font-size:2.2rem;line-height:1.15;letter-spacing:-.03em;margin:0 0 12px;overflow-wrap:anywhere}h2{font-size:1.4rem;line-height:1.3;margin:32px 0 12px}h3{font-size:1.1rem;margin:0 0 10px}p{max-width:72ch;margin:0 0 16px}.intro{font-size:1.08rem;max-width:64ch;margin-bottom:28px}.panel{background:#fff;border:var(--line);border-radius:12px;padding:22px;box-shadow:4px 4px 0 var(--ink);margin:0 4px 24px 0}.panel.accent{background:var(--mint)}.split{display:grid;grid-template-columns:minmax(0,1.3fr) minmax(260px,1fr);gap:28px}.stack>*+*{margin-top:16px}.row{display:flex;align-items:center;flex-wrap:wrap;gap:12px}.spread{justify-content:space-between}.muted{color:#514d53}.status{display:inline-block;border:var(--line);border-radius:6px;background:#eee7f8;padding:3px 9px;font-size:.9rem;font-weight:700;text-transform:capitalize}.preview{white-space:pre-wrap;overflow-wrap:anywhere;max-width:70ch;font-size:1.2rem;line-height:1.5}.small{font-size:.9rem}code{overflow-wrap:anywhere;font-size:.88rem}form{margin:16px 0}label{display:block;font-weight:650;margin:16px 0 6px}input:not([type=checkbox]),select,textarea{display:block;width:100%;max-width:720px;font:inherit;color:inherit;border:var(--line);border-radius:8px;padding:11px 12px;background:#fff;min-height:48px}textarea{min-height:130px;resize:vertical}input[type=checkbox]{width:22px;height:22px;flex-shrink:0;accent-color:#654298}.check{display:flex;align-items:start;gap:12px;font-weight:400;max-width:70ch;padding:8px 0}.check input{margin-top:2px}button,.button{display:inline-flex;align-items:center;justify-content:center;min-height:48px;padding:10px 17px;border:var(--line);border-radius:8px;background:var(--purple);color:var(--ink);box-shadow:3px 3px 0 var(--ink);font:650 1rem/1.3 ui-sans-serif,system-ui,sans-serif;text-decoration:none;cursor:pointer}button:hover,.button:hover{background:#c9b1f5}button:active,.button:active{transform:translate(2px,2px);box-shadow:1px 1px 0 var(--ink)}button.secondary,.button.secondary{background:#fff}button.danger{background:var(--pink)}button:disabled{background:#dedbd5;color:#5b5753;box-shadow:none;cursor:not-allowed}.error{padding:18px;border:var(--line);border-radius:10px;background:#ffd3e4;margin-bottom:20px}.notice{padding:16px;border:var(--line);border-radius:10px;background:#f5e09a;margin-bottom:22px}.success{padding:16px;border:var(--line);border-radius:10px;background:var(--mint);margin-bottom:22px}.table-scroll{overflow-x:auto;margin:18px 0 26px}table{border-collapse:collapse;width:100%;text-align:left;font-variant-numeric:tabular-nums}th,td{padding:12px;border-bottom:1px solid #a19b90;vertical-align:top}th{background:#eae3f4;font-size:.9rem}td form{margin:0}details{border:var(--line);border-radius:8px;padding:12px 16px;margin-bottom:16px}summary{cursor:pointer;font-weight:650;min-height:32px}.terms{font-size:.95rem;white-space:pre-wrap;margin-top:16px}.list{padding-left:22px}.list li{margin-bottom:10px}.empty{padding:24px 0;border-top:1px solid #aaa}footer{margin-top:36px;font-size:.85rem;color:#514d53}.skip{position:absolute;top:-60px;left:10px;background:#fff;padding:12px;z-index:10}.skip:focus{top:10px}@media(max-width:900px){.layout{grid-template-columns:175px minmax(0,1fr)}main{padding:26px 24px}.split{grid-template-columns:1fr}nav{padding:20px 10px}h1{font-size:1.9rem}}@media(max-width:600px){header{padding:14px 16px;align-items:start}.brand span{display:block;margin:0;font-size:.85rem}.layout{display:block}nav{display:flex;overflow-x:auto;border-right:0;border-bottom:var(--line);padding:12px;gap:8px}nav a{white-space:nowrap;margin:0}main{padding:24px 16px 50px}.panel{padding:18px}h1{font-size:1.75rem}h2{font-size:1.3rem}.row form{width:100%}button,.button{max-width:100%}.table-scroll{margin-left:-4px;margin-right:-4px}}@media(prefers-reduced-motion:reduce){button:active,.button:active{transform:none}}
</style></head><body><a class="skip" href="#main">Skip to content</a><header><a class="brand" href="{{if eq .Area "admin"}}/admin/{{else}}/portal/{{end}}">Knowoff<span>{{if eq .Area "admin"}}Operations{{else}}Contributor studio{{end}}</span></a>{{if .CSRF}}{{if .Area}}<form method="post" action="/{{.Area}}/logout"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><button class="secondary">Sign out</button></form>{{end}}{{end}}</header><div class="layout {{if not .Area}}solo{{end}}">{{if eq .Area "portal"}}<nav aria-label="Contributor navigation">{{range portalNav}}<a href="{{.URL}}" {{if eq $.Path .URL}}aria-current="page"{{end}}>{{.Label}}</a>{{end}}</nav>{{end}}{{if eq .Area "admin"}}<nav aria-label="Operations navigation">{{range adminNav}}<a href="{{.URL}}" {{if eq $.Path .URL}}aria-current="page"{{end}}>{{.Label}}</a>{{end}}</nav>{{end}}<main id="main"><h1>{{.Title}}</h1>{{if .Intro}}<p class="intro">{{.Intro}}</p>{{end}}{{template "body" .}}<footer>Knowoff · Make the table worth arguing about.</footer></main></div></body></html>{{end}}`

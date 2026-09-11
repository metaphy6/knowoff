package portal

import (
	"github.com/knowoff/knowoff/server/internal/webui"
	"net/http"
	"strings"
)

func portalPage(w http.ResponseWriter, r *http.Request, title, intro, body string, data any) {
	if body == challengeBody {
		if page, ok := data.(map[string]any); ok {
			page["WinnerID"] = challengeWinnerID(page["Snapshot"])
		}
	}

	webui.Render(w, 200, webui.Page{Title: title, Intro: intro, Area: "portal", Path: r.URL.Path, CSRF: csrfFromContext(r.Context()), Data: data}, body)
}

// The service snapshot carries a nullable winner pointer. Normalize it for
// template comparison without selecting a winner from votes or display order.
func challengeWinnerID(raw any) string {
	snapshot, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	topic, ok := snapshot["topic"].(map[string]any)
	if !ok {
		return ""
	}
	switch id := topic["winner_entry_id"].(type) {
	case *string:
		if id != nil {
			return *id
		}
	case string:
		return id
	}
	return ""
}

func portalError(w http.ResponseWriter, r *http.Request, err error, back string) {
	message := "We could not save that change. Reload the page and try again."
	for _, s := range []string{"terms", "contributor role", "daily submission cap", "only your own draft", "not owner", "not draft", "not withdrawable", "account level", "invalid role", "text content", "already", "challenge_full", "challenge_not_open", "challenge_self_vote", "screen"} {
		if strings.Contains(err.Error(), s) {
			message = err.Error()
			break
		}
	}
	webui.Render(w, 400, webui.Page{Title: "That change did not go through", Area: "portal", Path: back, CSRF: csrfFromContext(r.Context()), Data: map[string]string{"Message": message, "Back": back}}, `<div class="error" role="alert">{{.Data.Message}}</div><a class="button secondary" href="{{.Data.Back}}">Return to the form</a>`)
}

const termsBlock = `{{if .Data.Terms}}<details><summary>{{.Data.Terms.Title}} · {{.Data.Terms.Version}}</summary><div class="terms">{{.Data.Terms.Body}}</div></details>{{end}}`
const consentField = `<input type="hidden" name="terms_version" value="{{.Data.Terms.Version}}"><label class="check"><input type="checkbox" name="terms_accepted" value="yes" required><span>I accept contribution terms {{.Data.Terms.Version}}, including perpetual, non-exclusive commercial use and modification of my contribution.</span></label>`

const overviewBody = `<div class="split"><section><div class="panel accent"><h2 style="margin-top:0">Bring your wonderfully specific brain.</h2><p>Write something worth playing. Keep it surprising, readable, and yours.</p><a class="button" href="/portal/submissions">Open my submissions</a></div><h2>Your studio</h2><p>Active role: <span class="status">{{if .Data.Role}}{{.Data.Role}}{{else}}Player{{end}}</span></p><p>{{.Data.Submissions}} saved submissions. Approved work earns contributor credits and Noin through the server reward rules.</p><a href="/portal/apply">View applications and permissions</a>{{if .Data.CanSimulate}}<p style="margin-top:16px"><a href="/portal/simulate">Open the deal simulator</a></p>{{end}}</section><aside><h2 style="margin-top:0">From thought to table</h2><ol class="list"><li>Accept the current contribution terms and save a text draft.</li><li>Submit when it is ready. It becomes read-only in the queue.</li><li>Automated screening and a human decision come before publication.</li></ol><p class="muted">Text contributions are open here. Image processing, pack calls and deployment are still being built; an approval is not a published pack.</p><a href="/portal/challenge">See this week's community challenge</a></aside></div>`
const applicationBody = `<div class="split"><section><h2 style="margin-top:0">Choose how you contribute</h2><p>Your level: <strong>{{.Data.Level}}</strong>. Applications open at level <strong>{{.Data.MinLevel}}</strong>.</p><p>Current role: <span class="status">{{if .Data.Role}}{{.Data.Role}}{{else}}Player{{end}}</span></p><form method="post" action="/portal/apply"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><label for="role">Role</label><select id="role" name="role"><option value="contributor">Contributor — submit content</option><option value="curator">Curator — content review and authoring</option><option value="guard">Guard — community safety</option></select><p class="small muted">Curator authoring and Guard enforcement are still being built. A role grant does not enable unfinished tools.</p><button {{if not .Data.Eligible}}disabled{{end}}>Send application</button></form></section><section><h2 style="margin-top:0">Your applications</h2>{{range .Data.Applications}}<article class="panel"><div class="row spread"><h3>{{.Role}}</h3><span class="status">{{.Status}}</span></div><p class="small">Applied {{.AppliedAt.Format "02 Jan 2006"}}</p>{{if .Reason}}<p>{{.Reason}}</p>{{end}}</article>{{else}}<p class="empty">No application yet. Choose a role when your account is eligible.</p>{{end}}</section></div>`
const submissionsBody = `<div class="split"><section><h2 style="margin-top:0">Write a text contribution</h2>{{if .Data.CanContribute}}<p>A short, playable idea beats a paragraph explaining the joke.</p>` + termsBlock + `<form method="post" action="/portal/submissions"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><label for="new-content">Your text</label><textarea id="new-content" name="content" maxlength="{{.Data.MaxLength}}" required placeholder="The printer has requested annual leave."></textarea><p class="small muted">Up to {{.Data.MaxLength}} UTF-8 bytes. Save privately before sending for review.</p>` + consentField + `<button>Save draft</button></form>{{else}}<p class="notice">A Contributor role is required to write and submit. Apply first; the studio will be here when your role is approved.</p><a class="button" href="/portal/apply">View my application</a>{{end}}</section><section><h2 style="margin-top:0">Your submission history</h2>{{range .Data.Submissions}}<article class="panel"><div class="row spread"><span class="status">{{.Status}}</span><span class="small">Terms {{.TermsVersion}}</span></div><p class="preview" style="margin-top:16px">{{.Content}}</p>{{if .RejectionReason}}<p class="notice">Review note: {{.RejectionReason}}</p>{{end}}{{if eq .Status "draft"}}<details><summary>Edit this draft</summary><form method="post" action="/portal/submissions/{{.ID}}/edit"><input type="hidden" name="csrf_token" value="{{$.CSRF}}"><label for="content-{{.ID}}">Draft text</label><textarea id="content-{{.ID}}" name="content" maxlength="{{$.Data.MaxLength}}" required>{{.Content}}</textarea><button class="secondary">Save changes</button></form></details><form method="post" action="/portal/submissions/{{.ID}}/submit"><input type="hidden" name="csrf_token" value="{{$.CSRF}}"><p class="small muted">Submitting locks the text and uses one daily queue slot.</p><button>Submit for review</button></form>{{else if or (eq .Status "submitted") (eq .Status "in_review")}}<p class="small muted">Read-only while in review. Withdraw to edit; resubmitting uses another daily slot.</p><form method="post" action="/portal/submissions/{{.ID}}/withdraw"><input type="hidden" name="csrf_token" value="{{$.CSRF}}"><button class="secondary">Withdraw to draft</button></form>{{else if eq .Status "approved"}}<p class="small muted">Accepted for curation. Pack publication is a separate step.</p>{{end}}</article>{{else}}<p class="empty">No submissions yet. Your first draft belongs here.</p>{{end}}</section></div>`
const challengeBody = `{{if .Data.Snapshot.topic}}{{if .Data.Snapshot.topic.closed_at}}{{if .Data.WinnerID}}<p class="success">Week closed. The server has crowned the Week Winner below.</p>{{else}}<p class="notice">This week closed without a winner.</p>{{end}}{{end}}<div class="split"><section><div class="panel accent"><h2 style="margin-top:0">This week's Nown</h2><p class="preview">{{.Data.Snapshot.topic.nown.content}}</p><p class="small">{{.Data.Snapshot.topic.week_start.Format "02 Jan 2006"}} — {{.Data.Snapshot.topic.week_end.Format "02 Jan 2006"}}</p></div>{{if .Data.Snapshot.own_entry}}<h2>Your entry</h2><span class="status">{{.Data.Snapshot.own_entry.status}}</span><p class="preview">{{.Data.Snapshot.own_entry.content}}</p>{{if .Data.Snapshot.own_entry.rejection_reason}}<p>{{.Data.Snapshot.own_entry.rejection_reason}}</p>{{end}}{{else if .Data.Snapshot.can_submit}}<h2>One entry. Make it count.</h2><p>Entries cannot be edited or replaced after sending.</p>` + termsBlock + `<form method="post" action="/portal/challenge/entry"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><input type="hidden" name="topic_id" value="{{.Data.Snapshot.topic.id}}"><label for="entry">Your response</label><textarea id="entry" name="content" maxlength="{{.Data.MaxLength}}" required></textarea>` + consentField + `<button>Submit my entry</button></form>{{else}}<p class="notice">Entry intake is closed for this topic.</p>{{end}}</section><section><h2 style="margin-top:0">At the community table</h2>{{if .Data.Snapshot.voted_entry_id}}<p class="success">Your vote is recorded. It cannot be changed.</p>{{end}}{{range .Data.Snapshot.entries}}<article class="panel"><div class="row spread"><h3>{{if .nickname}}{{.nickname}}{{else}}Player{{end}}</h3>{{if and $.Data.Snapshot.topic.closed_at (eq .id $.Data.WinnerID)}}<span class="status">Week Winner</span>{{end}}</div><p class="preview">{{.content}}</p><p><strong>{{.vote_count}}</strong> votes {{if .is_own}}· Your entry{{end}}</p>{{if and $.Data.Snapshot.can_vote (not .is_own)}}<form method="post" action="/portal/challenge/vote"><input type="hidden" name="csrf_token" value="{{$.CSRF}}"><input type="hidden" name="entry_id" value="{{.id}}"><p class="small">Your one vote is final.</p><button>Vote for this entry</button></form>{{end}}</article>{{else}}<p class="empty">Nothing public yet. Entries appear only after automated screening and human approval.</p>{{end}}</section></div>{{else}}<div class="panel accent"><h2 style="margin-top:0">The next topic is on its way.</h2><p>No Weekly Nown Challenge is active. Check back here or in the game.</p><a href="/portal/">Return to the studio</a></div>{{end}}`
const simulatorBody = `<p>Curators can inspect the active pack's real dealt hands.</p><form method="post" action="/portal/simulate"><input type="hidden" name="csrf_token" value="{{.CSRF}}"><label for="table-size">Table size</label><select id="table-size" name="table_size"><option>4</option><option>6</option></select><label for="nown-id">Nown ID from the active pack</label><input id="nown-id" name="nown_id" required><label for="seed">Seed</label><input id="seed" name="seed" value="42" required><button>Deal test hands</button></form>`

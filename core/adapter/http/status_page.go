// core/adapter/http/status_page.go
package http

import (
	"io"
	"net/http"
	"os"

	"agentic-delegator/core/domain"
	"agentic-delegator/core/presenter/templ/pages"
	"agentic-delegator/core/usecase"
)

type StatusPage struct {
	get *usecase.GetJob
}

func NewStatusPage(get *usecase.GetJob) *StatusPage { return &StatusPage{get: get} }

// Render is GET /jobs/{id} — full HTML page.
func (p *StatusPage) Render(w http.ResponseWriter, r *http.Request, id string) {
	uid, _ := UserFromContext(r.Context())
	j, err := p.get.Execute(r.Context(), usecase.GetJobInput{JobID: domain.JobID(id), UserID: uid})
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	logTail := readLogTail(j.LogPath, 200)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.Status(j, logTail).Render(r.Context(), w)
}

// LogTail is GET /jobs/{id}/log — HTMX partial. Returns plain text.
func (p *StatusPage) LogTail(w http.ResponseWriter, r *http.Request, id string) {
	uid, _ := UserFromContext(r.Context())
	j, err := p.get.Execute(r.Context(), usecase.GetJobInput{JobID: domain.JobID(id), UserID: uid})
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(readLogTail(j.LogPath, 200)))
}

// maxLogReadBytes bounds how much of a (potentially huge) log file we pull into
// memory to render the tail.
const maxLogReadBytes = 256 << 10 // 256 KiB

func readLogTail(path string, maxLines int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.Size() > maxLogReadBytes {
		_, _ = f.Seek(-maxLogReadBytes, io.SeekEnd)
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	s := string(b)
	// crude tail — N=maxLines from the end
	lines := splitLines(s)
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return joinLines(lines)
}

func splitLines(s string) []string {
	out := []string{}
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func joinLines(ls []string) string {
	out := ""
	for i, l := range ls {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}

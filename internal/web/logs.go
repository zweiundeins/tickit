package web

import (
	"context"
	"log/slog"
	"strings"
)

// requestLog is the log of requests (edge.LogRequests) with what a path
// says of content taken out: a page's slug, which is its title, and a
// label's name (NFR-28).
func requestLog(log *slog.Logger) *slog.Logger {
	return slog.New(pathRedactor{log.Handler()})
}

// LogPath is p as the log of requests writes it.
func LogPath(p string) string {
	parts := strings.Split(p, "/")
	switch {
	case len(parts) == 6 && parts[1] == "spaces" && parts[3] == "pages" && !pageSubpaths[parts[5]]:
		parts[5] = "-"
	case len(parts) == 3 && parts[1] == "labels" && parts[2] != "":
		parts[2] = "-"
	default:
		return p
	}
	return strings.Join(parts, "/")
}

// pageSubpaths are the fixed paths below a page, which say nothing of it.
var pageSubpaths = map[string]bool{"history": true, "compare": true, "edit": true, "versions": true}

type pathRedactor struct{ slog.Handler }

func (h pathRedactor) Handle(ctx context.Context, r slog.Record) error {
	if r.Message != "request" {
		return h.Handler.Handle(ctx, r)
	}
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "path" {
			a.Value = slog.StringValue(LogPath(a.Value.String()))
		}
		out.AddAttrs(a)
		return true
	})
	return h.Handler.Handle(ctx, out)
}

func (h pathRedactor) WithAttrs(as []slog.Attr) slog.Handler {
	return pathRedactor{h.Handler.WithAttrs(as)}
}

func (h pathRedactor) WithGroup(name string) slog.Handler {
	return pathRedactor{h.Handler.WithGroup(name)}
}

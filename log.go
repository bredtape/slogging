package slogging

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
)

var (
	// ordered list of levels
	levels   = []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError}
	levelMap map[string]slog.Level
)

func init() {
	levelMap = make(map[string]slog.Level)
	for _, level := range levels {
		levelMap[strings.ToLower(level.String())] = level
	}
}

func LevelsString() string {
	xs := make([]string, 0, len(levels))
	for _, level := range levels {
		xs = append(xs, level.String())
	}
	return strings.Join(xs, ", ")
}

func ParseLevel(s string) (slog.Level, bool) {
	level, ok := levelMap[strings.ToLower(s)]
	return level, ok
}

// set default slog options and attributes
// returns a http Handler which can be used to get current log level and
// update it dynamically
func SetDefault(level slog.Level, addSource bool, jsonOutput bool, attributes ...slog.Attr) http.Handler {
	v := slog.LevelVar{}
	v.Set(level)

	opts := &slog.HandlerOptions{
		Level:     &v,
		AddSource: addSource}

	if jsonOutput {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, opts).WithAttrs(attributes)))
	} else {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, opts).WithAttrs(attributes)))
	}

	return logHandler{
		init:    level,
		current: &v}
}

type logHandler struct {
	init    slog.Level
	current *slog.LevelVar
}

func (h logHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Write([]byte(h.current.Level().String()))
	case http.MethodPut, http.MethodPost:
		// extract level from last path of URL
		xs := strings.Split(r.URL.Path, "/")
		if len(xs) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("specify log level as last part of the URL, e.g. PUT /log/debug"))
			return
		}
		x := xs[len(xs)-1]
		lvl, exists := ParseLevel(x)
		if !exists {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "unknown log level %q, specify one of %s", x, LevelsString())
			return
		}
		h.current.Set(lvl)
		w.WriteHeader(http.StatusAccepted)
		slog.Info("log level set", "newLevel", lvl)

	case http.MethodDelete:
		h.current.Set(h.init)
		w.WriteHeader(http.StatusAccepted)
		slog.Info("log level reset", "newLevel", h.init)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// log build info (go version, commit, date) at startup
// remember to build the application without specifying the .go file, e.g. "go build -o main", _not_ "go build -o main main.go"
func LogBuildInfo() {
	if info, ok := debug.ReadBuildInfo(); ok {
		var attrs []slog.Attr
		attrs = append(attrs, slog.String("goVersion", info.GoVersion))
		for _, kv := range info.Settings {
			if strings.HasPrefix(kv.Key, "vcs") {
				attrs = append(attrs, slog.String(kv.Key, kv.Value))
			}
		}
		slog.LogAttrs(context.Background(), slog.LevelInfo, "build info", attrs...)
	}
}

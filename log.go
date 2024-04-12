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

// will log to ERROR+4 and call os.Exit(1)
func Fatal(log *slog.Logger, message string, args ...any) {
	log.Log(context.Background(), slog.LevelError+4, message, args...)
	os.Exit(1)
}

// create logger with options and attributes
// returns a http Handler which can be used to get current log level and
// update it dynamically.
// the Handler must be mapped to a path prefix e.g. with gorilla mux:
// r := mux.NewRouter()
// r.PathPrefix("/log").Handler(logHandler)
func Create(opts slog.HandlerOptions, jsonOutput bool, attrs ...slog.Attr) (*slog.Logger, http.Handler) {
	v := slog.LevelVar{}
	v.Set(opts.Level.Level())

	o := &slog.HandlerOptions{
		Level:       &v,
		AddSource:   opts.AddSource,
		ReplaceAttr: opts.ReplaceAttr}

	h := logHandler{
		init:    opts.Level.Level(),
		current: &v}

	if jsonOutput {
		return slog.New(slog.NewJSONHandler(os.Stderr, o).WithAttrs(attrs)), h
	}
	return slog.New(slog.NewTextHandler(os.Stderr, o).WithAttrs(attrs)), h
}

// create logger (using Create) and sets the default logger
func SetDefaults(opts slog.HandlerOptions, jsonOutput bool, attributes ...slog.Attr) http.Handler {
	logger, handler := Create(opts, jsonOutput, attributes...)
	slog.SetDefault(logger)
	return handler
}

type logHandler struct {
	init    slog.Level
	current *slog.LevelVar
}

func (h logHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		_, _ = w.Write([]byte(h.current.Level().String()))
	case http.MethodPut, http.MethodPost:
		// extract level from last path of URL
		xs := strings.Split(r.URL.Path, "/")
		if len(xs) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("specify log level as last part of the URL, e.g. PUT /log/debug"))
			return
		}
		lastPart := xs[len(xs)-1]
		var lvl slog.Level
		err := lvl.UnmarshalText([]byte(lastPart))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "unknown log level %q", lastPart)
			return
		}
		h.current.Set(lvl)
		w.WriteHeader(http.StatusAccepted)
		slog.LogAttrs(context.Background(), slog.LevelInfo, "log level set", slog.String("newLevel", lvl.String()))

	case http.MethodDelete:
		h.current.Set(h.init)
		w.WriteHeader(http.StatusAccepted)
		slog.LogAttrs(context.Background(), slog.LevelInfo, "log level reset", slog.String("newLevel", h.init.String()))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// log build info (go version and vcs revision, time and modified) to Info level.
// Returns true if some build info was found.
// Remember to build the application without specifying the .go file,
// e.g. "go build -o main", _not_ "go build -o main main.go"
// See issue https://github.com/golang/go/issues/51279
func LogBuildInfo() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}

	var attrs []slog.Attr
	attrs = append(attrs, slog.String("goVersion", info.GoVersion))
	for _, kv := range info.Settings {
		if strings.HasPrefix(kv.Key, "vcs") {
			attrs = append(attrs, slog.String(kv.Key, kv.Value))
		}
	}
	slog.LogAttrs(context.Background(), slog.LevelInfo, "build info", attrs...)
	return true
}

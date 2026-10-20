//go:build pprof

package main

import (
	"net/http"
	"net/http/pprof"
)

// A binary built with -tags pprof serves /debug/pprof/ on its metrics
// listener (loopback only), for profiling under load (cmd/tickit-load).
func init() {
	debugHandlers = func(mux *http.ServeMux) {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}
}

package main

import (
	"net/http"
	"strings"
)

type PrefixRouter map[string]Handler

func (r PrefixRouter) Serve(t *Task) {
	prefix, suffix := t.Rq.URL.Path, ""

	if len(prefix) == 0 || prefix[0] != '/' {
		prefix = "/" + prefix
	}

	if i := strings.IndexRune(prefix[1:], '/'); i > -1 {
		suffix = prefix[i+1:]
		prefix = prefix[0 : i+1]
	} else {
		suffix = "/"
	}

	if handler, ok := r[prefix]; ok {
		t.Rq.URL.Path = suffix
		handler.Serve(t)
	} else if handler, ok := r["*"]; ok {
		handler.Serve(t)
	} else {
		t.Rw.WriteHeader(http.StatusNotFound)
	}
}

type MethodRouter map[string]Handler

func (r MethodRouter) Serve(t *Task) {
	if handler, ok := r[t.Rq.Method]; ok {
		handler.Serve(t)
	} else {
		verbs, i := make([]string, len(r)), 0
		for verb := range r {
			verbs[i] = verb
			i++
		}
		t.Rw.Header().Set("Allow", strings.Join(verbs, ", "))
		t.Rw.WriteHeader(http.StatusMethodNotAllowed)
	}
}

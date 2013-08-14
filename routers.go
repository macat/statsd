package main

import (
	"net/http"
	"strings"
)

type PrefixRouter map[string]Handler

func (r PrefixRouter) Serve(t *Task) {
	t.Rw.Header().Set("Access-Control-Allow-Origin", "*")
	prefix, suffix := t.Rq.URL.Path, ""

	for strings.Contains(prefix, "//") {
		prefix = strings.Replace(prefix, "//", "/", -1)
	}

	if len(prefix) == 0 || prefix[0] != '/' {
		prefix = "/" + prefix
	}

	if i := strings.IndexRune(prefix[1:], '/'); i > -1 {
		suffix = prefix[i+1:]
		prefix = prefix[0 : i+1]
	} else {
		suffix = "/"
	}

	if t.Rq.Method == "OPTIONS" {
		t.Rw.Header().Set("Access-Control-Allow-Origin", "*")
		t.Rw.Header().Set("Access-Control-Allow-Methods", "GET, POST")
		t.Rw.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept")
	} else if len(prefix) > 1 && prefix[1] == '*' {
		t.Rw.WriteHeader(http.StatusNotFound)
	} else if handler, ok := r[prefix]; ok {
		t.Rq.URL.Path = suffix
		handler.Serve(t)
	} else if handler, ok := r["*uuid"]; ok && ValidUUID(prefix[1:]) {
		t.Rq.URL.Path = suffix
		t.UUID = prefix[1:]
		handler.Serve(t)
	} else if handler, ok := r["*"]; ok {
		handler.Serve(t)
	} else {
		t.Rw.WriteHeader(http.StatusNotFound)
	}
}

type MethodRouter map[string]Handler

func (r MethodRouter) Serve(t *Task) {

	if t.Rq.URL.Path != "/" {
		t.Rw.WriteHeader(http.StatusNotFound)
	} else if handler, ok := r[t.Rq.Method]; ok {
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

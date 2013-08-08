package main

import "net/http"

type Static string

func (s Static) Serve(t *Task) {
	path := t.Rq.URL.Path
	if len(path) == 0 || path[0] != '/' {
		path = "/" + path
	}
	http.ServeFile(t.Rw, t.Rq, string(s)+path)
}

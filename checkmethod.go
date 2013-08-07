package main

import "net/http"

type CheckMethod struct {
	Method string
	Handler
}

func (h *CheckMethod) Serve(t *Task) {
	if t.Rq.Method != h.Method {
		t.Rw.Header().Set("Allow", h.Method)
		t.Rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	h.Handler.Serve(t)
}

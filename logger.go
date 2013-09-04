package main

import (
	"log"
	"time"
)

type Logger struct {
	Handler
}

func (l *Logger) Serve(t *Task) {
	t0 := time.Now()
	l.Handler.Serve(t)
	t1 := time.Now().Sub(t0)
	log.Println(t.Rw.StatusCode, t1, t.Rq.RequestURI)
}

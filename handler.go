package main

type Handler interface {
	Serve(*Task)
}

type HandlerFunc func(*Task)

func (f HandlerFunc) Serve(t *Task) {
	f(t)
}

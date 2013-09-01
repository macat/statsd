package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

type Task struct {
	Rw   http.ResponseWriter
	Rq   *http.Request
	Tx   *sql.Tx
	Uid  string
	UUID string
}

func (t *Task) SendJson(data interface{}) {
	t.Rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	t.Rw.Header().Set("Access-Control-Allow-Origin", "*")
	enc := json.NewEncoder(t.Rw)
	if err := enc.Encode(data); err != nil {
		panic(err)
	}
}

func (t *Task) SendJsonObject(name string, data interface{}) {
	jsonData := make(map[string]interface{})
	jsonData[name] = data
	t.SendJson(jsonData)
}

func (t *Task) SendError(msg string) {
	t.Rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	t.Rw.WriteHeader(http.StatusBadRequest)
	enc := json.NewEncoder(t.Rw)
	if err := enc.Encode(map[string]string{"msg": msg}); err != nil {
		panic(err)
	}
}

func (t *Task) RecvJson() interface{} {
	dec := json.NewDecoder(t.Rq.Body)
	var data interface{}
	if err := dec.Decode(&data); err != nil {
		return nil
	}
	return data
}

type Handler interface {
	Serve(*Task)
}

type HandlerFunc func(*Task)

func (f HandlerFunc) Serve(t *Task) {
	f(t)
}

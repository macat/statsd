package main

import (
	"net/http"
	"testing"
)

type TestHandler map[string]Handler

func (t TestHandler) Serve(task *Task) {
}

type TestUserChangerHandler map[string]Handler

func (t TestUserChangerHandler) Serve(task *Task) {
	task.Uid = "4b261947-6ae7-4f9c-9a5b-331a25336cc2"
}

func sessionTestwrap(sid string, test func(*Task)) {
	rq := http.Request{Header: http.Header{"Cookie": {"sid=" + sid}}}
	dbTest(&rq, test)
	if _, err := db.Exec(`DELETE FROM sessions`); err != nil {
		panic(err)
	}

}

func TestServe_new_session(t *testing.T) {
	sessionTestwrap("super", func(task *Task) {
		s := NewSession(TestHandler(map[string]Handler{}))
		s.Serve(task)

		headers := task.Rw.Header()

		if len(headers["Set-Cookie"]) != 1 {
			t.Errorf("It should set new session cookie")
		}
	})
}

func TestServe_login(t *testing.T) {
	sessionTestwrap("super", func(task *Task) {
		s := NewSession(TestUserChangerHandler(map[string]Handler{}))
		s.Serve(task)

		headers := task.Rw.Header()

		if len(headers["Set-Cookie"]) != 1 {
			t.Errorf("It should set new session cookie")
		}

		var uid string
		err := db.QueryRow(`
			SELECT "uid"
			FROM "sessions"
			ORDER BY created DESC
			LIMIT 1`).Scan(&uid)

		t.Log(err)

		if uid != "4b261947-6ae7-4f9c-9a5b-331a25336cc2" {
			t.Error("It should set session uid")
		}

	})
}

func TestServe_existing_session(t *testing.T) {
	sessionTestwrap("super-sid", func(task *Task) {
		uid := "88d181e2-de04-46f2-9901-925db3cea38a"

		_, err := db.Exec(`
			INSERT INTO "sessions" ("sid", "uid", "created")
			VALUES ('super-sid', $1, NOW())`, uid)
		if err != nil {
			panic(err)
		}

		s := NewSession(TestHandler(map[string]Handler{}))
		s.Serve(task)

		headers := task.Rw.Header()

		t.Log(headers)

		if len(headers) != 0 {
			t.Errorf("It should not set new session cookie")
		}

		if task.Uid != uid {
			t.Errorf("Task.Uid should be set.")
		}
	})
}

func TestServe_empty(t *testing.T) {
	sessionTestwrap("", func(task *Task) {
		uid := "88d181e2-de04-46f2-9901-925db3cea38a"

		_, err := db.Exec(`
			INSERT INTO "sessions" ("sid", "uid", "created")
			VALUES ('super-sid', $1, NOW())`, uid)
		if err != nil {
			panic(err)
		}

		s := NewSession(TestHandler(map[string]Handler{}))
		s.Serve(task)

		headers := task.Rw.Header()

		t.Log(headers)

		if len(headers) != 1 {
			t.Errorf("It should set new session cookie")
		}

		if task.Uid == uid {
			t.Errorf("Task.Uid should not be set.")
		}
	})
}

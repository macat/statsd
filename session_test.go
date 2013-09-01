package main

import (
	"database/sql"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

type TestHandler map[string]Handler

func (t TestHandler) Serve(task *Task) {
}

func dbTest(sid string, test func(*Task)) {
	var db *sql.DB
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	dbDriver := os.Getenv("DB_DRIVER")
	dbSetup := []string{
		"user=" + os.Getenv("DB_USER"),
		"dbname=" + os.Getenv("DB_NAME_TEST"),
		"sslmode=" + os.Getenv("DB_SSLMODE")}
	dsName := strings.Join(dbSetup, " ")

	if d, err := sql.Open(dbDriver, dsName); err != nil {
		log.Fatalln(err)
	} else {
		db = d
	}

	if err := db.Ping(); err != nil {
		log.Fatalln(err)
	}

	tx, err := db.Begin()
	if err != nil {
		panic(err)
	}
	rq := http.Request{Header: http.Header{"Cookie": {"sid=" + sid}}}
	rw := httptest.NewRecorder()
	test(&Task{Tx: tx, Rq: &rq, Rw: rw})
	defer tx.Rollback()
}

func TestServe_new_session(t *testing.T) {
	dbTest("super", func(task *Task) {
		s := NewSession(TestHandler(map[string]Handler{}))
		s.Serve(task)

		headers := task.Rw.Header()
		expecting := []string{"sid=" + s.sid + "; Path=/; HttpOnly"}

		if !reflect.DeepEqual(headers["Set-Cookie"], expecting) {
			t.Errorf("It should set new session cookie: %s; it set: %s",
				expecting, headers["Set-Cookie"])
		}
	})
}

func TestServe_existing_session(t *testing.T) {
	dbTest("super-sid", func(task *Task) {
		uid := "88d181e2-de04-46f2-9901-925db3cea38a"

		_, err := task.Tx.Exec(`
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

func TestServe_hacker(t *testing.T) {
	dbTest("", func(task *Task) {
		uid := "88d181e2-de04-46f2-9901-925db3cea38a"

		_, err := task.Tx.Exec(`
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

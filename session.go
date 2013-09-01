package main

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
)

const (
	SessionCookieName = "sid"
)

type Session struct {
	Handler
	sid string
	uid string
}

func NewSession(h Handler) *Session {
	return &Session{Handler: h}
}

func (s *Session) Serve(t *Task) {
	s.ensure(t)

	t.Uid = s.uid
	s.Handler.Serve(t)

	if t.Uid != s.uid {
		s.updateUid(t)
	}
}

func (s *Session) ensure(t *Task) {

	cookie, err := t.Rq.Cookie(SessionCookieName)

	if err != nil {
		s.startNew(t)
	} else {

		qerr := db.QueryRow(`
			SELECT "sid", "uid"
			FROM "sessions"
			WHERE "sid" = $1`,
			cookie.Value).Scan(&s.sid, &s.uid)

		if qerr != nil {
			s.startNew(t)
		}
	}
}

func (s *Session) updateUid(t *Task) {
	_, err := db.Exec(`UPDATE "sessions" SET uid = $1 WHERE sid = $2`, t.Uid, s.sid)
	if err != nil {
		panic(err)
	}
}

func (s *Session) startNew(t *Task) {
	bin := make([]byte, 18)
	if n, err := rand.Read(bin); err != nil {
		panic(err)
	} else if n < len(bin) {
		panic(io.EOF)
	}

	s.sid = base64.URLEncoding.EncodeToString(bin)
	_, err := db.Exec(`
		INSERT INTO "sessions" ("sid", "created")
		VALUES ($1, NOW())`,
		s.sid)

	if err != nil {
		panic(err)
	}

	http.SetCookie(t.Rw, &http.Cookie{
		Name:     SessionCookieName,
		Value:    s.sid,
		Path:     appRoot,
		HttpOnly: true,
	})
}

package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"io"
	"net/http"
)

const (
	SessionCookieName = "sid"
)

type Session struct {
	Handler
}

func NewSession(h Handler) *Session {
	return &Session{Handler: h}
}

func (s *Session) Serve(t *Task) {
	sid, uid := s.ensure(t)
	t.Uid = uid

	s.Handler.Serve(t)

	if t.Uid != uid {
		s.updateUid(t, sid)
	}
}

func (s *Session) ensure(t *Task) (sid, uid string) {

	uid = ""
	cookie, err := t.Rq.Cookie(SessionCookieName)

	if err != nil {
		sid = s.startNew(t)
	} else {

		var nullOrUid sql.NullString
		qerr := db.QueryRow(`
			SELECT "uid"
			FROM "sessions"
			WHERE "sid" = $1`,
			cookie.Value).Scan(&nullOrUid)
		switch {
		case qerr == sql.ErrNoRows:
			sid = s.startNew(t)
		case qerr != nil:
			panic(err)
		default:
			if nullOrUid.Valid {
				uid = nullOrUid.String
			}
			sid = cookie.Value
		}
	}
	return
}

func (s *Session) updateUid(t *Task, sid string) {
	if t.Uid != "" {
		_, err := db.Exec(`UPDATE "sessions" SET uid = $1 WHERE sid = $2`, t.Uid, sid)
		if err != nil {
			panic(err)
		}
	}
}

func (s *Session) startNew(t *Task) (sid string) {
	bin := make([]byte, 18)
	if n, err := rand.Read(bin); err != nil {
		panic(err)
	} else if n < len(bin) {
		panic(io.EOF)
	}

	sid = base64.URLEncoding.EncodeToString(bin)
	_, err := db.Exec(`
		INSERT INTO "sessions" ("sid", "created")
		VALUES ($1, NOW())`,
		sid)

	if err != nil {
		panic(err)
	}

	http.SetCookie(t.Rw, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sid,
		Path:     appRoot,
		HttpOnly: true,
	})
	return
}

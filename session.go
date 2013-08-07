package main

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	SessionCookieName      = "sid"
	SessionExpiry          = 2 * time.Hour
	SessionCleanupInterval = 1 * time.Minute
)

type Session struct {
	Handler
	sync.Mutex
	entries map[string]*sessionEntry
}

type sessionEntry struct {
	uid       string
	idleSince time.Time
}

func NewSession(h Handler) *Session {
	return &Session{Handler: h, entries: make(map[string]*sessionEntry)}
}

func (s *Session) Serve(t *Task) {
	sid, uid := s.ensureSession(t)

	t.Uid = uid
	s.Handler.Serve(t)

	if t.Uid != uid {
		s.Lock()
		defer s.Unlock()
		s.entries[sid].uid = t.Uid
	}
}

func (s *Session) ensureSession(t *Task) (string, string) {
	newSession, sid, entry, ok := false, "", (*sessionEntry)(nil), false

	cookie, err := t.Rq.Cookie(SessionCookieName)

	s.Lock()
	defer s.Unlock()

	if err != nil {
		newSession = true
	} else {
		sid = cookie.Value
		entry, ok = s.entries[sid]
		if !ok || time.Now().Sub(entry.idleSince) >= SessionExpiry {
			newSession = true
		}
	}

	if newSession {
		bin := make([]byte, 18)
		if n, err := rand.Read(bin); err != nil {
			panic(err)
		} else if n < len(bin) {
			panic(io.EOF)
		}

		sid = base64.URLEncoding.EncodeToString(bin)
		entry = &sessionEntry{"", time.Now()}

		s.entries[sid] = entry
		if len(s.entries) == 1 {
			go s.cleanup()
		}

		http.SetCookie(t.Rw, &http.Cookie{
			Name:     SessionCookieName,
			Value:    sid,
			Path:     appRoot,
			HttpOnly: true,
		})
	} else {
		entry.idleSince = time.Now()
	}

	return sid, entry.uid
}

func (s *Session) cleanup() {
	for {
		time.Sleep(SessionCleanupInterval)
		deadline := time.Now().Add(-SessionExpiry)

		s.Lock()
		for k, v := range s.entries {
			if v.idleSince.Before(deadline) {
				delete(s.entries, k)
			}
		}

		if len(s.entries) == 0 {
			break
		} else {
			s.Unlock()
		}

	}
	s.Unlock()
}

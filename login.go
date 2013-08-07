package main

import "net/http"

func login(t *Task) {
	t.Uid = "412d5b90-fd23-11e2-98f5-14dae9e93c91" // TODO
}

func logout(t *Task) {
	t.Uid = ""
	t.Rw.WriteHeader(http.StatusNoContent)
}

func whoami(t *Task) {
	if !ValidUUID(t.Uid) {
		t.Rw.WriteHeader(http.StatusNotFound)
		return
	}

	row := t.Tx.QueryRow(`SELECT COUNT(*) FROM "users" WHERE "id" = $1`, t.Uid)
	n := 0
	if err := row.Scan(&n); err != nil {
		panic(err)
	}

	if n < 1 {
		t.Rw.WriteHeader(http.StatusNotFound)
		return
	}

	t.Rw.Header().Set("Location", "/users/"+t.Uid)
	t.Rw.WriteHeader(http.StatusFound)
}

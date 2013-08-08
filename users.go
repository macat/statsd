package main

import (
	"net/http"
	"time"
	"database/sql"
)

var usersRouter = &Transactional{PrefixRouter(map[string]Handler{
	"/": MethodRouter(map[string]Handler{
		"GET":  HandlerFunc(listUsers),
		"POST": HandlerFunc(createUser),
	}),
	"*": MethodRouter(map[string]Handler{
		"GET":    HandlerFunc(getUser),
		"PATCH":  HandlerFunc(changeUser),
		"DELETE": HandlerFunc(deleteUser),
	}),
})}

func listUsers(t *Task) {
	rows, err := t.Tx.Query(`SELECT "id", "name", "email", "created" FROM "users"`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	id, name, email, created := "", "", "", time.Time{}
	users := make([]map[string]string, 0)
	for rows.Next() {
		if err := rows.Scan(&id, &name, &email, &created); err != nil {
			panic(err)
		}
		users = append(users, map[string]string{
			"id":      id,
			"name":    name,
			"email":   email,
			"created": created.Format("2006-01-02 15:04:05"),
		})
	}

	t.SendJson(users)
}

func createUser(t *Task) {
	data, ok := t.RecvJson().(map[string]interface{})
	if !ok {
		t.Rw.WriteHeader(http.StatusBadRequest)
		return
	}

	name, ok := data["name"].(string)
	if !ok || name == "" {
		t.SendError("'name' is required")
		return
	}

	email, ok := data["email"].(string) // TODO: validate email
	if !ok || email == "" {
		t.SendError("'email' is required")
		return
	}

	if emailUsed(t, email) != "" {
		t.Rw.WriteHeader(http.StatusConflict)
		return
	}

	id, err := NewUUID4()
	if err != nil {
		panic(err)
	}

	_, err = t.Tx.Exec(`
		INSERT INTO "users" ("id", "name", "email", "created")
		VALUES ($1, $2, $3, NOW())`,
		id, name, email)

	if err != nil {
		panic(err)
	}

	t.Rw.WriteHeader(http.StatusCreated)
	t.SendJson(map[string]string{"id": id})
}

func getUser(t *Task) {
	uid := t.Rq.URL.Path[1:]
	if !ValidUUID(uid) {
		t.Rw.WriteHeader(http.StatusNotFound)
		return
	}

	rows, err := t.Tx.Query(`SELECT * FROM "users" WHERE "id" = $1`, uid)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Rw.WriteHeader(http.StatusNotFound)
		return
	}

	id, name, email, created := "", "", "", time.Time{}
	if err := rows.Scan(&id, &name, &email, &created); err != nil {
		panic(err)
	}

	t.SendJson(map[string]string{
		"id":      id,
		"name":    name,
		"email":   email,
		"created": created.Format("2006-01-02 15:04:06"),
	})
}

func changeUser(t *Task) {
	uid := userExists(t)
	if len(uid) == 0 {
		return
	}

	data, ok := t.RecvJson().(map[string]interface{})
	if !ok {
		t.Rw.WriteHeader(http.StatusBadRequest)
		return
	}

	fields := map[string]interface{}{}

	if name, ok := data["name"].(string); ok {
		if name == "" {
			t.SendError("'name' is required")
			return
		}
		fields["name"] = name
	}

	if email, ok := data["email"].(string); ok { // TODO: validate email
		if email == "" {
			t.SendError("'email' is required")
			return
		}
		if usedBy := emailUsed(t, email); usedBy != "" && usedBy != uid {
			t.Rw.WriteHeader(http.StatusConflict)
			return
		}
		fields["email"] = email
	}

	if len(fields) > 0 {
		set, vals := setClause(fields, uid)
		_, err := t.Tx.Exec(`UPDATE "users" `+set+` WHERE "id" = $1`, vals...)

		if err != nil {
			panic(err)
		}
	}

	t.Rw.WriteHeader(http.StatusNoContent)
}

func deleteUser(t *Task) {
	uid := userExists(t)
	if len(uid) == 0 {
		return
	}

	_, err := t.Tx.Exec(`DELETE FROM "users" WHERE "id" = $1`, uid)
	if err != nil {
		panic(err)
	}

	t.Rw.WriteHeader(http.StatusNoContent)
}

func userExists(t *Task) string {
	uid, n := t.Rq.URL.Path[1:], 0
	if !ValidUUID(uid) {
		t.Rw.WriteHeader(http.StatusNotFound)
		return ""
	}

	row := t.Tx.QueryRow(`SELECT COUNT(*) FROM "users" WHERE "id" = $1`, uid)
	if err := row.Scan(&n); err != nil {
		panic(err)
	}

	if n < 1 {
		t.Rw.WriteHeader(http.StatusNotFound)
		return ""
	}

	return uid
}

func emailUsed(t *Task, email string) string {
	row := t.Tx.QueryRow(`SELECT "id" FROM "users" WHERE "email" = $1`, email)
	uid := ""
	if err := row.Scan(&uid); err != nil {
		if err != sql.ErrNoRows {
			panic(err)
		}
	}
	return uid
}

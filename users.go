package main

import (
	"code.google.com/p/go.crypto/bcrypt"
	"database/sql"
	"net/http"
	"time"
)

var usersRouter = &Transactional{PrefixRouter(map[string]Handler{
	"/": MethodRouter(map[string]Handler{
		"GET":  HandlerFunc(listUsers),
		"POST": HandlerFunc(createUser),
	}),
	"*uuid": PrefixRouter(map[string]Handler{
		"/": MethodRouter(map[string]Handler{
			"GET":    HandlerFunc(getUser),
			"PATCH":  HandlerFunc(changeUser),
			"DELETE": HandlerFunc(deleteUser),
		}),
	}),
})}

func listUsers(t *Task) {
	whereClause1, whereClause2, params := "", "", []interface{}{}

	gid := t.Rq.URL.Query().Get("group")
	if len(gid) > 0 {
		if !groupExists(t.Tx, gid) {
			t.SendJson([]int{})
			return
		}
		params = append(params, gid)
		subq := `SELECT "user_id" FROM "users_to_groups" WHERE "group_id" = $1`
		whereClause1 = `WHERE "id" IN (` + subq + `)`
		whereClause2 = `WHERE "user_id" IN (` + subq + `)`
	}

	rows, err := t.Tx.Query(`
		SELECT "id", "name", "email", "created"
		FROM "users" `+whereClause1, params...)

	if err != nil {
		panic(err)
	}
	defer rows.Close()

	uid, name, email, created := "", "", "", time.Time{}
	users := make([]map[string]interface{}, 0)
	uids2indexes := make(map[string]int, 0)
	for rows.Next() {
		if err := rows.Scan(&uid, &name, &email, &created); err != nil {
			panic(err)
		}
		uids2indexes[uid] = len(users)
		users = append(users, map[string]interface{}{
			"id":          uid,
			"name":        name,
			"email":       email,
			"created":     created.Format("2006-01-02 15:04:05"),
			"groups":      make([]string, 0),
			"permissions": make([]string, 0),
		})
	}

	rows, err = t.Tx.Query(`
		SELECT "user_id", "group_id"
		FROM "users_to_groups"`+whereClause2,
		params...)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	uid, gid = "", ""
	for rows.Next() {
		if err := rows.Scan(&uid, &gid); err != nil {
			panic(err)
		}
		user := users[uids2indexes[uid]]
		user["groups"] = append(user["groups"].([]string), gid)
	}

	rows, err = t.Tx.Query(`
		SELECT DISTINCT "user_id", UNNEST("permissions")
		FROM "groups"
		JOIN "users_to_groups" ON "id" = "group_id"`+whereClause2,
		params...)
	if err != nil {
		panic(nil)
	}
	defer rows.Close()

	for rows.Next() {
		perm := ""
		if err := rows.Scan(&uid, &perm); err != nil {
			panic(err)
		}
		user := users[uids2indexes[uid]]
		user["permissions"] = append(user["permissions"].([]string), perm)
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

	email, ok := data["email"].(string)
	if !ok || email == "" {
		t.SendError("'email' is required")
		return
	}
	if !emailRegexp.MatchString(email) {
		t.SendError("'email' is invalid")
		return
	}
	if emailUsed(t.Tx, email) != "" {
		t.Rw.WriteHeader(http.StatusConflict)
		return
	}

	passwdStr, ok := data["password"].(string)
	if !ok || passwdStr == "" {
		t.SendError("'password' is required")
		return
	}
	if len(passwdStr) < 8 {
		t.SendError("'password' is too short")
		return
	}
	passwd := []byte(passwdStr)
	hash, err := bcrypt.GenerateFromPassword(passwd, bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}

	id, err := NewUUID4()
	if err != nil {
		panic(err)
	}

	_, err = t.Tx.Exec(`
		INSERT INTO "users" ("id", "name", "email", "created", "password")
		VALUES ($1, $2, $3, NOW(), $4)`,
		id, name, email, string(hash))

	if err != nil {
		panic(err)
	}

	t.Rw.WriteHeader(http.StatusCreated)
	t.SendJson(map[string]string{"id": id})
}

func getUser(t *Task) {
	rows, err := t.Tx.Query(`
		SELECT "id", "name", "email", "created"
		FROM "users"
		WHERE "id" = $1`,
		t.UUID)

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
	rows.Close()

	user := map[string]interface{}{
		"id":          id,
		"name":        name,
		"email":       email,
		"created":     created.Format("2006-01-02 15:04:06"),
		"groups":      groupsOfUser(t.Tx, id),
		"permissions": userPermissions(t.Tx, id),
	}

	t.SendJson(user)
}

func changeUser(t *Task) {
	if !userExists(t.Tx, t.UUID) {
		t.Rw.WriteHeader(http.StatusNotFound)
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

	if email, ok := data["email"].(string); ok {
		if email == "" {
			t.SendError("'email' is required")
			return
		}
		if !emailRegexp.MatchString(email) {
			t.SendError("'email' is invalid")
			return
		}
		if usedBy := emailUsed(t.Tx, email); usedBy != "" && usedBy != t.UUID {
			t.Rw.WriteHeader(http.StatusConflict)
			return
		}
		fields["email"] = email
	}

	if passwdStr, ok := data["password"].(string); ok {
		if passwdStr == "" {
			t.SendError("'password' is invalid")
			return
		}
		if len(passwdStr) < 8 {
			t.SendError("'password' is too short")
			return
		}
		passwd := []byte(passwdStr)
		hash, err := bcrypt.GenerateFromPassword(passwd, bcrypt.DefaultCost)
		if err != nil {
			panic(err)
		}
		fields["password"] = string(hash)
	}

	if len(fields) > 0 {
		set, vals := setClause(fields, t.UUID)
		_, err := t.Tx.Exec(`UPDATE "users" `+set+` WHERE "id" = $1`, vals...)

		if err != nil {
			panic(err)
		}
	}
}

func deleteUser(t *Task) {
	result, err := t.Tx.Exec(`DELETE FROM "users" WHERE "id" = $1`, t.UUID)
	if err != nil {
		panic(err)
	}

	if n, err := result.RowsAffected(); err != nil {
		panic(err)
	} else if n == 0 {
		t.Rw.WriteHeader(http.StatusNotFound)
		return
	}
}

func userExists(tx *sql.Tx, uid string) bool {
	if !ValidUUID(uid) {
		return false
	}

	row := tx.QueryRow(`SELECT COUNT(*) FROM "users" WHERE "id" = $1`, uid)
	n := 0
	if err := row.Scan(&n); err != nil {
		panic(err)
	}

	return n > 0
}

func emailUsed(tx *sql.Tx, email string) string {
	row := tx.QueryRow(`SELECT "id" FROM "users" WHERE "email" = $1`, email)
	uid := ""
	if err := row.Scan(&uid); err != nil {
		if err != sql.ErrNoRows {
			panic(err)
		}
	}
	return uid
}

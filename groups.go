package main

import (
	"database/sql"
	"net/http"
	"time"
)

var groupsRouter = &Transactional{PrefixRouter(map[string]Handler{
	"/": MethodRouter(map[string]Handler{
		"GET":  HandlerFunc(listGroups),
		"POST": HandlerFunc(createGroup),
	}),
	"*uuid": PrefixRouter(map[string]Handler{
		"/": MethodRouter(map[string]Handler{
			"GET":    HandlerFunc(getGroup),
			"PATCH":  HandlerFunc(changeGroup),
			"DELETE": HandlerFunc(deleteGroup),
		}),
		"/users": MethodRouter(map[string]Handler{
			"POST":   HandlerFunc(addUserToGroup),
			"DELETE": HandlerFunc(removeUserFromGroup),
		}),
	}),
})}

func listGroups(t *Task) {
	rows, err := t.Tx.Query(`SELECT "id", "name", "created" FROM "groups"`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	id, name, created := "", "", time.Time{}
	groups := make([]map[string]interface{}, 0)
	gids2indexes := make(map[string]int)
	for rows.Next() {
		if err := rows.Scan(&id, &name, &created); err != nil {
			panic(err)
		}
		gids2indexes[id] = len(groups)
		groups = append(groups, map[string]interface{}{
			"id":          id,
			"name":        name,
			"created":     created.Format("2006-01-02 15:04:05"),
			"permissions": make([]string, 0),
		})
	}

	rows, err = t.Tx.Query(`
		SELECT "id", UNNEST("permissions")
		FROM "groups"`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		perm := ""
		if err := rows.Scan(&id, &perm); err != nil {
			panic(err)
		}
		group := groups[gids2indexes[id]]
		group["permissions"] = append(group["permissions"].([]string), perm)
	}

	t.SendJson(groups)
}

func createGroup(t *Task) {
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

	id, err := NewUUID4()
	if err != nil {
		panic(err)
	}

	_, err = t.Tx.Exec(`INSERT INTO "groups" ("id", "name", "created")
		VALUES ($1, $2, NOW())`,
		id, name)

	if err != nil {
		panic(err)
	}

	t.Rw.WriteHeader(http.StatusCreated)
	t.SendJson(map[string]string{"id": id})
}

func getGroup(t *Task) {
	rows, err := t.Tx.Query(`
		SELECT "id", "name", "created"
		FROM "groups"
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

	id, name, created := "", "", time.Time{}
	if err := rows.Scan(&id, &name, &created); err != nil {
		panic(err)
	}
	rows.Close()

	group := map[string]interface{}{
		"id":          id,
		"name":        name,
		"created":     created.Format("2006-01-02 15:04:06"),
		"permissions": make([]string, 0),
	}

	rows, err = t.Tx.Query(`
		SELECT UNNEST("permissions") FROM "groups"
		WHERE "id" = $1`, id)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		perm := ""
		if err := rows.Scan(&perm); err != nil {
			panic(err)
		}
		group["permissions"] = append(group["permissions"].([]string), perm)
	}

	t.SendJson(group)
}

func changeGroup(t *Task) {
	if !groupExists(t.Tx, t.UUID) {
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

	if len(fields) > 0 {
		set, vals := setClause(fields, t.UUID)
		_, err := t.Tx.Exec(`UPDATE "groups" `+set+` WHERE "id" = $1`, vals...)

		if err != nil {
			panic(err)
		}
	}
}

func deleteGroup(t *Task) {
	result, err := t.Tx.Exec(`DELETE FROM "groups" WHERE "id" = $1`, t.UUID)
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

func addUserToGroup(t *Task) {
	if !groupExists(t.Tx, t.UUID) {
		t.Rw.WriteHeader(http.StatusNotFound)
		return
	}

	uid, ok := t.RecvJson().(string)
	if !ok || !userExists(t.Tx, uid) {
		t.SendError("Invalid user ID")
		return
	}

	if userInGroup(t.Tx, uid, t.UUID) {
		return
	}

	_, err := t.Tx.Exec(
		`INSERT INTO "users_to_groups" ("user_id", "group_id")
		VALUES ($1, $2)`,
		uid, t.UUID)
	if err != nil {
		panic(err)
	}
}

func removeUserFromGroup(t *Task) {
	if !groupExists(t.Tx, t.UUID) {
		t.Rw.WriteHeader(http.StatusNotFound)
		return
	}

	uid, ok := t.RecvJson().(string)
	if !ok || !userExists(t.Tx, uid) {
		t.SendError("Invalid user ID")
		return
	}

	_, err := t.Tx.Exec(`
		DELETE FROM "users_to_groups"
		WHERE user_id = $1 AND group_id = $2`,
		uid, t.UUID)

	if err != nil {
		panic(err)
	}
}

func addPermission(t *Task) {
	// TODO
}

func removePermission(t *Task) {
	// TODO
}

func groupExists(tx *sql.Tx, gid string) bool {
	if !ValidUUID(gid) {
		return false
	}

	row := tx.QueryRow(`SELECT COUNT(*) FROM "groups" WHERE "id" = $1`, gid)
	n := 0
	if err := row.Scan(&n); err != nil {
		panic(err)
	}

	return n > 0
}

func userInGroup(tx *sql.Tx, uid, gid string) bool {
	if !ValidUUID(uid) || !ValidUUID(gid) {
		return false
	}

	row := tx.QueryRow(`
		SELECT COUNT(*) FROM "users_to_groups"
		WHERE "user_id" = $1 AND "group_id" = $2`, uid, gid)

	n := 0
	if err := row.Scan(&n); err != nil {
		panic(err)
	}

	return n > 0
}

func groupsOfUser(tx *sql.Tx, uid string) []string {
	if !ValidUUID(uid) {
		return nil
	}

	rows, err := tx.Query(`
		SELECT "group_id" FROM "users_to_groups"
		WHERE "user_id" = $1`, uid)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	groups := make([]string, 0)
	for rows.Next() {
		group := ""
		if err := rows.Scan(&group); err != nil {
			panic(err)
		}
		groups = append(groups, group)
	}

	return groups
}

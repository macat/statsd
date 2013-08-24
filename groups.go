package main

import (
	"admin/uuids"
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
	if !hasPermission(t.Tx, t.Uid, "GET", "groups", "") {
		t.Rw.WriteHeader(http.StatusForbidden)
		return
	}

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
			"permissions": make([]map[string]string, 0),
		})
	}

	rows, err = t.Tx.Query(`
		SELECT "group_id", "method", "object_type", "object_id"
		FROM "permissions"`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var method, objType, objId []byte
		if err := rows.Scan(&id, &method, &objType, &objId); err != nil {
			panic(err)
		}
		group := groups[gids2indexes[id]]
		perm := map[string]string{}
		if method != nil {
			perm["method"] = string(method)
		} else {
			perm["method"] = ""
		}
		if objType != nil {
			perm["type"] = string(objType)
		} else {
			perm["type"] = ""
		}
		if objId != nil {
			perm["id"] = string(objId)
		} else {
			perm["id"] = ""
		}
		group["permissions"] = append(group["permissions"].([]map[string]string),
			perm)
	}

	t.SendJson(groups)
}

func createGroup(t *Task) {
	if !hasPermission(t.Tx, t.Uid, "POST", "groups", "") {
		t.Rw.WriteHeader(http.StatusForbidden)
		return
	}

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

	id, err := uuids.NewUUID4()
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
	if !hasPermission(t.Tx, t.Uid, "GET", "group", t.UUID) {
		t.Rw.WriteHeader(http.StatusForbidden)
		return
	}

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
		"permissions": make([]map[string]string, 0),
	}

	rows, err = t.Tx.Query(`
		SELECT "method", "object_type", "object_id"
		FROM "permissions"
		WHERE "group_id" = $1`,
		id)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var method, objType, objId []byte
		if err := rows.Scan(&method, &objType, &objId); err != nil {
			panic(err)
		}
		perm := map[string]string{}
		if method != nil {
			perm["method"] = string(method)
		} else {
			perm["method"] = ""
		}
		if objType != nil {
			perm["type"] = string(objType)
		} else {
			perm["type"] = ""
		}
		if objId != nil {
			perm["id"] = string(objId)
		} else {
			perm["id"] = ""
		}
		group["permissions"] = append(group["permissions"].([]map[string]string),
			perm)
	}

	t.SendJson(group)
}

func changeGroup(t *Task) {
	if !hasPermission(t.Tx, t.Uid, "PATCH", "group", t.UUID) {
		t.Rw.WriteHeader(http.StatusForbidden)
		return
	}

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
	if !hasPermission(t.Tx, t.Uid, "DELETE", "group", t.UUID) {
		t.Rw.WriteHeader(http.StatusForbidden)
		return
	}

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

	_, err = t.Tx.Exec(`
		DELETE FROM "permissions"
		WHERE "object_type" = 'group' AND "object_id" = $1`,
		t.UUID)
	if err != nil {
		panic(err)
	}
}

func addUserToGroup(t *Task) {
	if !hasPermission(t.Tx, t.Uid, "POST", "group_members", t.UUID) {
		t.Rw.WriteHeader(http.StatusForbidden)
		return
	}

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
	if !hasPermission(t.Tx, t.Uid, "DELETE", "group_members", t.UUID) {
		t.Rw.WriteHeader(http.StatusForbidden)
		return
	}

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

func groupExists(tx *sql.Tx, gid string) bool {
	if !uuids.ValidUUID(gid) {
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
	if !uuids.ValidUUID(uid) || !uuids.ValidUUID(gid) {
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
	if !uuids.ValidUUID(uid) {
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

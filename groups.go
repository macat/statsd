package main

import (
	"net/http"
	"time"
)

var groupsRouter = &Transactional{PrefixRouter(map[string]Handler{
	"/": MethodRouter(map[string]Handler{
		"GET":  HandlerFunc(listGroups),
		"POST": HandlerFunc(createGroup),
	}),
	"*": MethodRouter(map[string]Handler{
		"GET":    HandlerFunc(getGroup),
		"PATCH":  HandlerFunc(changeGroup),
		"DELETE": HandlerFunc(deleteGroup),
	}),
})}

func listGroups(t *Task) {
	rows, err := t.Tx.Query(`SELECT "id", "name", "created" FROM "groups"`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	id, name, created := "", "", time.Time{}
	groups := make([]map[string]string, 0)
	for rows.Next() {
		if err := rows.Scan(&id, &name, &created); err != nil {
			panic(err)
		}
		groups = append(groups, map[string]string{
			"id":      id,
			"name":    name,
			"created": created.Format("2006-01-02 15:04:05"),
		})
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
	gid := t.Rq.URL.Path[1:]
	if !ValidUUID(gid) {
		t.Rw.WriteHeader(http.StatusNotFound)
		return
	}

	rows, err := t.Tx.Query(`SELECT * FROM "groups" WHERE "id" = $1`, gid)
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

	t.SendJson(map[string]string{
		"id":      id,
		"name":    name,
		"created": created.Format("2006-01-02 15:04:06"),
	})
}

func changeGroup(t *Task) {
	gid := groupExists(t)
	if len(gid) == 0 {
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
		set, vals := setClause(fields, gid)
		_, err := t.Tx.Exec(`UPDATE "groups" `+set+` WHERE "id" = $1`, vals...)

		if err != nil {
			panic(err)
		}
	}

	t.Rw.WriteHeader(http.StatusNoContent)
}

func deleteGroup(t *Task) {
	gid := groupExists(t)
	if len(gid) == 0 {
		return
	}

	_, err := t.Tx.Exec(`DELETE FROM "groups" WHERE "id" = $1`, gid)
	if err != nil {
		panic(err)
	}

	t.Rw.WriteHeader(http.StatusNoContent)
}

func groupExists(t *Task) string {
	gid, n := t.Rq.URL.Path[1:], 0
	if !ValidUUID(gid) {
		t.Rw.WriteHeader(http.StatusNotFound)
		return ""
	}

	row := t.Tx.QueryRow(`SELECT COUNT(*) FROM "groups" WHERE "id" = $1`, gid)
	if err := row.Scan(&n); err != nil {
		panic(err)
	}

	if n < 1 {
		t.Rw.WriteHeader(http.StatusNotFound)
		return ""
	}

	return gid
}

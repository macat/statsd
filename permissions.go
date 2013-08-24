package main

import (
	"admin/uuids"
	"database/sql"
	"net/http"
	"strconv"
)

var permissionsRouter = &Transactional{MethodRouter(map[string]Handler{
	"POST":   HandlerFunc(grantPermission),
	"DELETE": HandlerFunc(revokePermission),
})}

func grantPermission(t *Task) {
	if !hasPermission(t.Tx, t.Uid, "POST", "permissions", "") {
		t.Rw.WriteHeader(http.StatusForbidden)
		return
	}

	data, ok := t.RecvJson().(map[string]interface{})

	if !ok {
		t.Rw.WriteHeader(http.StatusBadRequest)
		return
	}

	group, ok := data["group"].(string)
	if !ok || !groupExists(t.Tx, group) {
		t.SendError("Invalid 'group'")
		return
	}

	method, ok := data["method"].(string)
	if !ok {
		t.SendError("Invalid 'method'")
		return
	}

	objType, ok := data["type"].(string)
	if !ok {
		t.SendError("Invalid 'type'")
		return
	}

	objId, ok := data["id"].(string)
	if !ok || objId != "" && !uuids.ValidUUID(objId) {
		t.SendError("Invalid 'id'")
		return
	}

	fields := map[string]interface{}{"group_id": group}
	if method != "" {
		fields["method"] = method
	}
	if objType != "" {
		fields["object_type"] = objType
	}
	if objId != "" {
		fields["object_id"] = objId
	}
	insert, vals := insertClause(fields)
	_, err := t.Tx.Exec(`INSERT INTO "permissions" `+insert, vals...)
	if err != nil {
		panic(err)
	}
}

func revokePermission(t *Task) {
	if !hasPermission(t.Tx, t.Uid, "DELETE", "permissions", "") {
		t.Rw.WriteHeader(http.StatusForbidden)
		return
	}

	data, ok := t.RecvJson().(map[string]interface{})

	if !ok {
		t.Rw.WriteHeader(http.StatusBadRequest)
		return
	}

	group, ok := data["group"].(string)
	if !ok || !groupExists(t.Tx, group) {
		t.SendError("Invalid 'group'")
		return
	}

	method, ok := data["method"].(string)
	if !ok {
		t.SendError("Invalid 'method'")
		return
	}

	objType, ok := data["type"].(string)
	if !ok {
		t.SendError("Invalid 'type'")
		return
	}

	objId, ok := data["id"].(string)
	if !ok {
		t.SendError("Invalid 'id'")
		return
	}

	where, vals := `WHERE "group_id" = $1`, []interface{}{group}
	if method != "" {
		vals = append(vals, method)
		where += ` AND "method" = $` + strconv.Itoa(len(vals))
	} else {
		where += ` AND "method" IS NULL`
	}
	if objType != "" {
		vals = append(vals, objType)
		where += ` AND "object_type" = $` + strconv.Itoa(len(vals))
	} else {
		where += ` AND "object_type" IS NULL`
	}
	if objId != "" {
		vals = append(vals, objId)
		where += ` AND "object_id" = $` + strconv.Itoa(len(vals))
	} else {
		where += ` AND "object_id" IS NULL`
	}

	_, err := t.Tx.Exec(`DELETE FROM "permissions" `+where, vals...)
	if err != nil {
		panic(err)
	}
}

func hasPermission(tx *sql.Tx, uid, method, objType, objId string) bool {
	if !uuids.ValidUUID(uid) {
		return false
	}

	objIdQ, params := "", []interface{}{uid, method, objType}
	if objId != "" {
		if uuids.ValidUUID(objId) {
			objIdQ = `"object_id" = $4 OR`
			params = append(params, objId)
		} else {
			return false
		}
	}
	row := tx.QueryRow(`
		SELECT COUNT(*)
		FROM "permissions"
		WHERE
			"group_id" IN (
				SELECT "group_id"
				FROM "users_to_groups"
				WHERE user_id = $1
			) AND
			("method" = $2 OR "method" IS NULL) AND
			("object_type" = $3 OR "object_type" IS NULL) AND
			(`+objIdQ+` "object_id" IS NULL)
		`,
		params...)

	n := 0
	err := row.Scan(&n)
	if err != nil {
		return false
	}

	return n > 0
}

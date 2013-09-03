package main

import (
	"admin/access"
	"admin/uuid"
	"net/http"
	"strconv"
)

var permissionsRouter = &Transactional{MethodRouter(map[string]Handler{
	"POST":   HandlerFunc(grantPermission),
	"DELETE": HandlerFunc(revokePermission),
})}

func grantPermission(t *Task) {
	if !access.HasPermission(t.Tx, t.Uid, "POST", "permissions", "") {
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
	if !ok || objId != "" && !uuid.Valid(objId) {
		t.SendError("Invalid 'id'")
		return
	}

	access.Grant(t.Tx, group, method, objType, objId)
}

func revokePermission(t *Task) {
	if !access.HasPermission(t.Tx, t.Uid, "DELETE", "permissions", "") {
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

package main

import (
	"database/sql"
)

var permissionsRouter = &Transactional{MethodRouter(map[string]Handler{
	"POST":   HandlerFunc(grantPermissions),
	"DELETE": HandlerFunc(revokePermissions),
})}

func grantPermissions(t *Task) {
	// TODO
}

func revokePermissions(t *Task) {
	// TODO
}

func userPermissions(tx *sql.Tx, uid string) []string {
	perms := make([]string, 0)
	if !ValidUUID(uid) {
		return perms
	}

	rows, err := tx.Query(`
		SELECT DISTINCT UNNEST("permissions")
		FROM "groups"
		WHERE "id" IN (
			SELECT "group_id"
			FROM "users_to_groups"
			WHERE "user_id" = $1
		)`,
		uid)

	if err != nil {
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		perm := ""
		if err := rows.Scan(&perm); err != nil {
			panic(err)
		}
		perms = append(perms, perm)
	}

	return perms
}

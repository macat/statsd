package main

import (
	"code.google.com/p/go.crypto/bcrypt"
	"database/sql"
	"net/http"
)

func login(t *Task) {
	data, ok := t.RecvJson().(map[string]interface{})
	if !ok {
		t.SendJson(map[string]bool{"success": false})
		return
	}

	email, uid, ok := "", "", false
	var passwd, hash []byte

	if email, ok = data["email"].(string); !ok {
		t.SendJson(map[string]bool{"success": false})
		return
	}

	if p, ok := data["password"].(string); !ok {
		t.SendJson(map[string]bool{"success": false})
		return
	} else {
		passwd = []byte(p)
	}

	row := t.Tx.QueryRow(`
		SELECT "id", "password" FROM "users" 
		WHERE "email" = $1`,
		email)

	if err := row.Scan(&uid, &hash); err != nil {
		if err == sql.ErrNoRows {
			t.SendJson(map[string]bool{"success": false})
			return
		} else {
			panic(err)
		}
	}

	err := bcrypt.CompareHashAndPassword(hash, passwd)
	if err == nil {
		t.Uid = uid

		// If the password was encrypted with a lower cost factor than the
		// current default, rehash it on the first successful login attempt:
		cost, err := bcrypt.Cost(hash)
		if err != nil {
			panic(err)
		}

		if cost < bcrypt.DefaultCost {
			hash, err := bcrypt.GenerateFromPassword(passwd, bcrypt.DefaultCost)
			if err != nil {
				panic(err)
			}

			_, err = t.Tx.Exec(`
				UPDATE "users"
				SET "password" = $1
				WHERE "id" = $2`,
				hash, uid)

			if err != nil {
				panic(err)
			}
		}
	}
	if err == nil {
		http.SetCookie(t.Rw, &http.Cookie{Name: "uid", Value: uid})
		t.SendJson(map[string]interface{}{"success": true, "id": uid})
	} else {
		t.SendJson(map[string]interface{}{"success": false})
	}
}

func logout(t *Task) {
	t.Uid = ""
	t.SendJsonObject("success", true)
}

func whoami(t *Task) {
	if !userExists(t.Tx, t.Uid) {
		t.SendJson(map[string]string{"id": ""})
	} else {
		t.SendJson(map[string]string{"id": t.Uid})
	}
}

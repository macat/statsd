package main

import (
	"code.google.com/p/go.crypto/bcrypt"
	"database/sql"
)

func login(t *Task) {
	data, ok := t.RecvJson().(map[string]interface{})
	if !ok {
		t.SendJson(false)
		return
	}

	email, uid, ok := "", "", false
	var passwd, hash []byte

	if email, ok = data["email"].(string); !ok {
		t.SendJson(false)
		return
	}

	if p, ok := data["password"].(string); !ok {
		t.SendJson(false)
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
			t.SendJson(false)
		} else {
			panic(err)
		}
	}

	err := bcrypt.CompareHashAndPassword(hash, passwd)
	if err == nil {
		t.Uid = uid

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

	t.SendJson(err == nil)
}

func logout(t *Task) {
	t.Uid = ""
}

func whoami(t *Task) {
	if !userExists(t.Tx, t.Uid) {
		t.SendJson(nil)
	} else {
		t.SendJson(t.Uid)
	}
}

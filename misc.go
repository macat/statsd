package main

import (
	"regexp"
	"strconv"
)

func setClause(x map[string]interface{}, p ...interface{}) (string, []interface{}) {
	n, str, values := 0, "SET ", make([]interface{}, len(x)+len(p))
	copy(values, p)
	for name, value := range x {
		if n > 0 {
			str += `, `
		}
		values[len(p)+n] = value
		n++
		str += `"` + name + `" = $` + strconv.Itoa(len(p)+n)
	}

	return str, values
}

func insertClause(x map[string]interface{}, p ...interface{}) (string, []interface{}) {
	str1, str2, values := "(", ") VALUES ("
	n, values := 0, make([]interface{}, len(x)+len(p))

	copy(values, p)
	for name, value := range x {
		if n > 0 {
			str1 += ", "
			str2 += ", "
		}
		values[len(p)+n] = value
		n++
		str1 += `"` + name + `"`
		str2 += "$" + strconv.Itoa(len(p)+n)
	}
	return str1 + str2 + ")", values
}

var emailRegexp = regexp.MustCompile(".+@.+\\..")

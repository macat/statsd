package main

import (
	"crypto/rand"
	"io"
)

func NewUUID4() (string, error) {
	var (
		rnd  [16]byte
		buff [36]byte
	)

	if n, err := rand.Read(rnd[:]); err != nil {
		return "", err
	} else if n < len(rnd) {
		return "", io.EOF
	}

	rnd[6] = rnd[6]&0x0f | 0x40
	rnd[8] = rnd[8]&0x3f | 0x80

	var digits = "0123456789abcdef"
	for s, d := 0, 0; s < len(rnd); s++ {
		l, h := rnd[s]&0x0f, rnd[s]>>4
		buff[d], buff[d+1] = digits[h], digits[l]
		switch s {
		case 3, 5, 7, 9:
			buff[d+2] = '-'
			d += 3
		default:
			d += 2
		}
	}

	return string(buff[:]), nil
}

func ValidUUID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, ch := range id {
		switch i {
		case 8, 13, 18, 23:
			if ch != '-' {
				return false
			}
		default:
			if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
				return false
			}
		}
	}
	return true
}

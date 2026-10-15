package ticketing

import (
	"crypto/rand"
	"regexp"
)

// codeAlphabet omits characters that are easy to confuse when read aloud or
// printed: 0/O, 1/I/L.
const codeAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

var codePattern = regexp.MustCompile(`^OPS-[` + codeAlphabet + `]{4}-[` + codeAlphabet + `]{4}$`)

// NewCode returns a random ticket code like "OPS-7KQ2-M9XA".
// 31^8 ≈ 8.5e11 combinations; the tickets primary key catches the rare collision.
func NewCode() string {
	var b [8]byte
	for i := range b {
		b[i] = codeAlphabet[randIndex(len(codeAlphabet))]
	}
	return "OPS-" + string(b[:4]) + "-" + string(b[4:])
}

// ValidCode reports whether s has the ticket code format.
func ValidCode(s string) bool {
	return codePattern.MatchString(s)
}

// randIndex returns a uniform random int in [0, n) using crypto/rand with
// rejection sampling (no modulo bias).
func randIndex(n int) int {
	limit := 256 - 256%n
	var b [1]byte
	for {
		if _, err := rand.Read(b[:]); err != nil {
			panic(err) // crypto/rand never fails on supported platforms
		}
		if int(b[0]) < limit {
			return int(b[0]) % n
		}
	}
}

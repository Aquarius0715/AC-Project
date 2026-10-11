// Package logging builds the JSON logger of every process (backend Go design §8: log/slog). Its handler masks what must
// never reach a log line, even when code passes it by mistake (IR369):
//   - an attribute whose key names a secret (*secret*, *token*, *password*, *authorization*, *cookie*);
//   - e-mail addresses, phone numbers in international form (+60…) and the password of a connection URL, inside any
//     message, string or error.
package logging

import (
	"io"
	"log/slog"
	"regexp"
	"strings"
)

// Masked stands in for a value the handler hides.
const Masked = "[masked]"

var (
	email   = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	phone   = regexp.MustCompile(`\+\d[\d \-]{6,16}\d`)
	userURL = regexp.MustCompile(`(://[^/:@\s]+:)[^@\s]+@`)
)

// secretKey reports whether an attribute's key names a secret.
func secretKey(k string) bool {
	k = strings.ToLower(k)
	for _, s := range []string{"secret", "token", "password", "authorization", "cookie"} {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// Mask hides the e-mail addresses, international phone numbers and connection-URL passwords in a text.
func Mask(s string) string {
	s = userURL.ReplaceAllString(s, "${1}"+Masked+"@")
	s = email.ReplaceAllString(s, Masked)
	return phone.ReplaceAllString(s, Masked)
}

// ReplaceAttr masks one attribute (slog.HandlerOptions.ReplaceAttr): the value of a secret-named key, and the personal
// data and passwords inside a string or an error.
func ReplaceAttr(_ []string, a slog.Attr) slog.Attr {
	if secretKey(a.Key) {
		return slog.String(a.Key, Masked)
	}
	switch a.Value.Kind() {
	case slog.KindString:
		if s := a.Value.String(); Mask(s) != s {
			return slog.String(a.Key, Mask(s))
		}
	case slog.KindAny:
		if err, ok := a.Value.Any().(error); ok {
			return slog.String(a.Key, Mask(err.Error()))
		}
	}
	return a
}

// New is the JSON logger of one service, writing to w.
func New(w io.Writer, service string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{ReplaceAttr: ReplaceAttr})).With("service", service)
}

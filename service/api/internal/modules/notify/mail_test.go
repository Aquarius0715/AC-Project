package notify

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
)

// TestMailTexts: a notification e-mail names its template and target and links to the recipient's app.
func TestMailTexts(t *testing.T) {
	apps := Apps{Customer: "http://c", Partner: "http://p", Technician: "http://t", Admin: "http://a"}
	subject, body := notificationMail("alert", map[string]any{"targetName": "Bedroom AC", "status": "open"}, "client", apps)
	if subject != "Alert: Bedroom AC" || !strings.Contains(body, "Status: open") || !strings.Contains(body, "http://c/notifications") {
		t.Errorf("notification: %q %q", subject, body)
	}
	if s, _ := notificationMail("alert", map[string]any{"targetName": "Bed\r\nroom AC"}, "client", apps); s != "Alert: Bed room AC" { // a name with a line break
		t.Errorf("subject on one line: %q", s)
	}
	if s, _ := notificationMail("unknown", nil, "admin", apps); s != "Notification: unknown" {
		t.Errorf("unknown template: %q", s)
	}
}

// TestSMTPRefusesHeaderBreaks: an address or subject with a line break never reaches the SMTP server.
func TestSMTPRefusesHeaderBreaks(t *testing.T) {
	s := SMTP{Addr: "127.0.0.1:1", From: "x@example.com"}
	for _, c := range [][2]string{{"a@example.com\r\nBcc: b@example.com", "Hi"}, {"a@example.com", "Hi\nBcc: b@example.com"}} {
		if _, err := s.Send(context.Background(), c[0], c[1], "body"); err == nil || !strings.Contains(err.Error(), "line break") {
			t.Errorf("%q: %v", c, err)
		}
	}
}

// TestSMTPSends: a message reaches an SMTP server with the bare sender address in the envelope and the display name
// in the From header, a Date, the non-ASCII subject RFC 2047 encoded and CRLF line ends in the body.
func TestSMTPSends(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan string, 1)
	go func() { // a minimal SMTP server: accept one message and report its envelope sender and data
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		w := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }
		w("220 test")
		var from string
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					w("250 ok")
					got <- from + "\n" + data.String()
					continue
				}
				data.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				w("250 test")
			case strings.HasPrefix(cmd, "MAIL FROM:"):
				from = strings.TrimSpace(line[len("MAIL FROM:"):])
				w("250 ok")
			case strings.HasPrefix(cmd, "RCPT TO:"):
				w("250 ok")
			case cmd == "DATA":
				inData = true
				w("354 go")
			case cmd == "QUIT":
				w("221 bye")
				return
			default:
				w("250 ok")
			}
		}
	}()
	s := SMTP{Addr: ln.Addr().String(), From: "AC Project <no-reply@ac-project.local>"}
	if _, err := s.Send(context.Background(), "tan@example.com", "Hello 陈", "Body line\r\nsecond"); err != nil {
		t.Fatal(err)
	}
	m := <-got
	if !strings.HasPrefix(m, "<no-reply@ac-project.local>") || !strings.Contains(m, "From: AC Project <no-reply@ac-project.local>") || !strings.Contains(m, "Subject: =?UTF-8?q?Hello_=E9=99=88?=\r\n") ||
		!strings.Contains(m, "\r\nDate: ") || !strings.Contains(m, "Body line\r\nsecond") || strings.Contains(m, "\r\r\n") {
		t.Errorf("received: %q", m)
	}
}

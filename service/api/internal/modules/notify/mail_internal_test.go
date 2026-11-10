package notify

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/platform/authz"
)

// TestMailTexts: a notification e-mail names its template and target and links to the recipient's app.
func TestMailTexts(t *testing.T) {
	t.Parallel()

	apps := Apps{
		Customer:   "http://c",
		Partner:    "http://p",
		Technician: "http://t",
		Admin:      "http://a",
		Hosts:      nil,
	}

	subject, body := notificationMail(
		noticeAlert,
		map[string]any{"targetName": "Bedroom AC", keyStatus: "open"},
		authz.RoleClient,
		"",
		apps,
	)
	if subject != "Alert: Bedroom AC" || !strings.Contains(body, "Status: open") ||
		!strings.Contains(body, "http://c/notifications") {
		t.Errorf("notification: %q %q", subject, body)
	}

	if got, _ := notificationMail(
		noticeAlert,
		map[string]any{"targetName": "Bed\r\nroom AC"},
		authz.RoleClient,
		"",
		apps,
	); got != "Alert: Bed room AC" { // a name with a line break
		t.Errorf("subject on one line: %q", got)
	}

	if got, _ := notificationMail(
		"unknown",
		nil,
		authz.RoleAdmin,
		"",
		apps,
	); got != "Notification: unknown" {
		t.Errorf("unknown template: %q", got)
	}

	for role, app := range map[string]string{
		authz.RoleClient: "http://c", authz.RoleContractor: "http://p", authz.RoleTechnician: "http://t",
		authz.RoleAdmin: "http://a",
	} {
		// each role's own app
		if _, body := notificationMail(
			noticeAlert,
			nil,
			role,
			"",
			apps,
		); !strings.Contains(
			body,
			app+"/notifications",
		) {
			t.Errorf("%s links to %q", role, body)
		}
	}
}

// TestMailLinksAtTheSignInAddress: an environment on several addresses links each e-mail to the address its recipient
// last signed in on, when that address is listed (IR721, B-16); any other, or none, keeps the canonical one.
func TestMailLinksAtTheSignInAddress(t *testing.T) {
	t.Parallel()

	apps := Apps{
		Customer:   "http://10.100.1.17:3200",
		Partner:    "http://10.100.1.17:3201",
		Technician: "http://10.100.1.17:3202",
		Admin:      "http://10.100.1.17:3203",
		Hosts:      []string{"10.100.1.17", "100.99.169.116"},
	}

	const canonical = "http://10.100.1.17:3200/notifications"

	for host, want := range map[string]string{
		"100.99.169.116": "http://100.99.169.116:3200/notifications",
		"10.100.1.17":    canonical,
		"evil.example":   canonical,
		"":               canonical,
	} {
		if _, body := notificationMail(
			noticeAlert,
			nil,
			authz.RoleClient,
			host,
			apps,
		); !strings.Contains(
			body,
			want,
		) {
			t.Errorf("signed in on %q: %q", host, body)
		}
	}

	single := Apps{
		Customer:   "http://localhost:3000",
		Partner:    "",
		Technician: "",
		Admin:      "",
		Hosts:      nil,
	}
	if _, body := notificationMail(
		noticeAlert,
		nil,
		authz.RoleClient,
		"100.99.169.116",
		single,
	); !strings.Contains(
		body,
		"http://localhost:3000/notifications",
	) { // one address (dev, CI, E2E): no listed host, nothing moves
		t.Errorf("single address: %q", body)
	}
}

// TestSMTPRefusesHeaderBreaks: an address or subject with a line break never reaches the SMTP server.
func TestSMTPRefusesHeaderBreaks(t *testing.T) {
	t.Parallel()

	s := SMTP{Addr: "127.0.0.1:1", From: "x@example.com"}
	for _, pair := range [][2]string{
		{"a@example.com\r\nBcc: b@example.com", "Hi"},
		{"a@example.com", "Hi\nBcc: b@example.com"},
	} {
		_, err := s.Send(
			context.Background(),
			pair[0],
			pair[1],
			"body",
		)
		if err == nil ||
			!strings.Contains(err.Error(), "line break") {
			t.Errorf("%q: %v", pair, err)
		}
	}
}

// TestSMTPSends: a message reaches an SMTP server with the bare sender address in the envelope and the display name
// in the From header, a Date, the non-ASCII subject RFC 2047 encoded and CRLF line ends in the body.
func TestSMTPSends(t *testing.T) {
	t.Parallel()

	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()

	got := make(chan string, 1)

	go func() { // a minimal SMTP server: accept one message and report its envelope sender and data
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		reader := bufio.NewReader(conn)
		reply := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		reply("220 test")

		var (
			from string
			data strings.Builder
		)

		inData := false

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}

			if inData {
				if line == ".\r\n" {
					inData = false

					reply("250 ok")

					got <- from + "\n" + data.String()

					continue
				}

				data.WriteString(line)

				continue
			}

			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				reply("250 test")
			case strings.HasPrefix(cmd, "MAIL FROM:"):
				from = strings.TrimSpace(line[len("MAIL FROM:"):])

				reply("250 ok")
			case strings.HasPrefix(cmd, "RCPT TO:"):
				reply("250 ok")
			case cmd == "DATA":
				inData = true

				reply("354 go")
			case cmd == "QUIT":
				reply("221 bye")

				return
			default:
				reply("250 ok")
			}
		}
	}()

	s := SMTP{Addr: listener.Addr().String(), From: "AC Project <no-reply@ac-project.local>"}

	_, sendErr := s.Send(
		context.Background(),
		"tan@example.com",
		"Hello 陈", //nolint:gosmopolitan // a Chinese name checks the UTF-8 encoding of the subject
		"Body line\r\nsecond",
	)
	if sendErr != nil {
		t.Fatal(sendErr)
	}

	message := <-got
	if !strings.HasPrefix(message, "<no-reply@ac-project.local>") ||
		!strings.Contains(message, "From: AC Project <no-reply@ac-project.local>") ||
		!strings.Contains(message, "Subject: =?UTF-8?q?Hello_=E9=99=88?=\r\n") ||
		!strings.Contains(message, "\r\nDate: ") ||
		!strings.Contains(message, "Body line\r\nsecond") ||
		strings.Contains(message, "\r\r\n") {
		t.Errorf("received: %q", message)
	}
}

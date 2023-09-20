package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/smtp"
	"os"
	"text/template"
	"time"
)

var (
	addr = os.Getenv("REMINDER_SMTP_ADDR")
	user = os.Getenv("REMINDER_SMTP_USER")
	pass = os.Getenv("REMINDER_SMTP_PASS")
	t    = template.Must(template.New("reminder").Parse("From: {{.From}}\r\nTo: {{.To}}\r\nSubject: {{.Subject}}\r\nContent-Type: {{.ContentType}}\r\n\r\n{{.Body}}"))
)

type Event struct {
	Title string `json:"title"`
	Time  string `json:"time"`
}

type Manifest struct {
	Events []*Event `json:"events"`
}

func main() {
	if len(os.Args) < 2 {
		slog.Error("no manifest")
		os.Exit(1)
	}

	file := os.Args[1]
	b, err := os.ReadFile(file)
	if err != nil {
		slog.Error("read manifest", "err", err, "file", file)
		os.Exit(1)
	}
	var m Manifest
	err = json.Unmarshal(b, &m)
	if err != nil {
		slog.Error("unmarshal manifest", "err", err, "file", file)
		os.Exit(1)
	}

	now := time.Now().Local()
	for _, e := range m.Events {
		t, err := time.ParseInLocation(time.DateOnly, e.Time, now.Location())
		if err != nil {
			slog.Error("parse event time", "err", err, "title", e.Title, "time", e.Time)
			continue
		}
		t = t.AddDate(0, 0, 1)
		if t.Before(now) {
			slog.Warn("event is expired", "title", e.Title, "time", e.Time)
			continue
		}
		day := int(t.Sub(now).Hours() / 24)
		slog.Info("event", "title", e.Title, "time", e.Time, "day", day)
		if day <= 7 {
			notification(e, day)
		}
	}
}

func notification(event *Event, day int) {
	type Data struct {
		From        string
		To          string
		Subject     string
		ContentType string
		Body        string
		Event       *Event
	}

	if addr == "" {
		slog.Warn("send notification skip", "reason", "addr is empty")
		return
	}
	slog.Info("sending notification")
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		slog.Error("send notification fail", "err", err)
		return
	}

	body := ""
	subject := "事件提醒"
	if day > 0 {
		body += fmt.Sprintf("距离「%s」还有%d天\n\n", event.Title, day)
	} else {
		body += fmt.Sprintf("今天「%s」\n\n", event.Title)
	}
	data := Data{
		From:        fmt.Sprintf("%s <%s>", mime.BEncoding.Encode("UTF-8", "Monitor"), user),
		To:          user,
		Subject:     mime.BEncoding.Encode("UTF-8", fmt.Sprintf("「RED」%s", subject)),
		ContentType: "text/plain; charset=utf-8",
		Body:        body,
		Event:       event,
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		slog.Error("send notification fail", "err", err)
		return
	}

	auth := smtp.PlainAuth("", user, pass, host)
	if err := smtp.SendMail(addr, auth, user, []string{user}, buf.Bytes()); err != nil {
		slog.Error("send notification fail", "err", err)
		return
	}
	slog.Info("send notification success")
}

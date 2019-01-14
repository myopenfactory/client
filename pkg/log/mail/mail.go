package mail

import (
	"encoding/json"
	"fmt"
	"net/smtp"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// EmailSender declares a method which transmits an email.
type EmailSender interface {
	Send(to []string, body []byte) error
}

// MailHook to sends logs by email with authentication.
type MailHook struct {
	AppName  string
	Address  string
	From     string
	To       string
	Username string
	Password string
	send     func(string, smtp.Auth, string, []string, []byte) error
	levels   []logrus.Level
}

// NewMailHook creates a MailHook and configures it from parameters.
func New(appname, address, sender, receiver, username, password, level string) *MailHook {
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		panic(err)
	}
	return &MailHook{
		AppName:  appname,
		Address:  address,
		From:     sender,
		To:       receiver,
		Username: username,
		Password: password,
		send:     smtp.SendMail,
		levels: []logrus.Level{
			logLevel,
		},
	}
}

// Fire is called when a log event is fired.
func (h *MailHook) Fire(entry *logrus.Entry) error {
	if entry == nil {
		return nil
	}

	body := fmt.Sprintf("%s-%s", entry.Time.Format(time.RFC3339Nano), entry.Message)
	subject := fmt.Sprintf("%s-%s", h.AppName, entry.Level)
	fields, _ := json.MarshalIndent(entry.Data, "", "\t")
	message := fmt.Sprintf("Subject: %s\r\n\r\n%s\r\n\r\n%s", subject, body, fields)

	err := h.sendMail([]string{h.To}, []byte(message))
	return errors.Wrapf(err, "failed sending log mail")
}

func (h *MailHook) sendMail(to []string, body []byte) error {
	var auth smtp.Auth
	if h.Username != "" {
		auth = smtp.PlainAuth("", h.Username, h.Password, h.Address)
	}
	return h.send(h.Address, auth, h.From, to, body)
}

// Levels returns the available logging levels.
func (h *MailHook) Levels() []logrus.Level {
	return h.levels
}

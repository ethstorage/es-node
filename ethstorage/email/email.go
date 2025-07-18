// Copyright 2022-2023, EthStorage.
// For license information, see https://github.com/ethstorage/es-node/blob/main/LICENSE
package email

import (
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethstorage/go-ethstorage/ethstorage/p2p"
)

type EmailConfig struct {
	Username string
	Password string
	Host     string
	Port     uint64
	To       []string
	From     string
}

func (c EmailConfig) Check() error {
	if c.Username == "" {
		return fmt.Errorf("email username is empty")
	}
	if c.Password == "" {
		return fmt.Errorf("email password is empty")
	}
	if c.Host == "" {
		return fmt.Errorf("email host is empty")
	}
	if c.Port == 0 {
		return fmt.Errorf("email port is empty")
	}
	if len(c.To) == 0 {
		return fmt.Errorf("email to is empty")
	}
	if c.From == "" {
		return fmt.Errorf("email from is empty")
	}
	return nil
}

func (c EmailConfig) String() string {
	return fmt.Sprintf(
		"Username: %s\nPassword: %s\nHost: %s\nPort: %d\nTo: %s\nFrom: %s",
		c.Username,
		strings.Repeat("*", len(c.Password)),
		c.Host,
		c.Port,
		strings.Join(c.To, ", "),
		c.From,
	)
}

func SendEmail(emailSubject, msg string, config EmailConfig, lg log.Logger) {
	lg.Info("Sending email notification", "subject", emailSubject)

	emailBody := fmt.Sprintf("Subject: %s\r\n", emailSubject)
	emailBody += fmt.Sprintf("To: %s\r\n", strings.Join(config.To, ", "))
	emailBody += fmt.Sprintf("From: \"EthStorage\" <%s>\r\n", config.From)
	localIP := p2p.GetLocalPublicIPv4()
	if localIP != nil {
		msg = strings.Replace(msg, "es-node", fmt.Sprintf("the es-node at %s", localIP.String()), 1)
	}
	msg += "\n\nSent at: " + time.Now().Format(time.RFC1123Z)
	emailBody += "\r\n" + msg
	lg.Debug("Email body", "body", emailBody)

	err := smtp.SendMail(
		fmt.Sprintf("%s:%d", config.Host, config.Port),
		smtp.PlainAuth("", config.Username, config.Password, config.Host),
		config.From,
		config.To,
		[]byte(emailBody),
	)
	if err != nil {
		lg.Error("Failed to send email", "error", err, "config", config.String())
	} else {
		lg.Info("Email notification sent successfully!")
	}
}

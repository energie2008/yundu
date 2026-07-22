package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
)

// SMTPSender 使用 Go 标准库 net/smtp 发送邮件
// 支持 STARTTLS（端口 587/25）和隐式 TLS（端口 465）
type SMTPSender struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// NewSMTPSender 构造 SMTPSender
func NewSMTPSender(host string, port int, username, password, from string) *SMTPSender {
	return &SMTPSender{
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		From:     from,
	}
}

// Send 发送 HTML 邮件
func (s *SMTPSender) Send(ctx context.Context, to string, subject string, htmlBody string) error {
	from := s.From
	if from == "" {
		from = s.Username
	}

	headers := make(map[string]string)
	headers["From"] = from
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=UTF-8"

	var msgParts []string
	for k, v := range headers {
		msgParts = append(msgParts, fmt.Sprintf("%s: %s", k, v))
	}
	msgParts = append(msgParts, "", htmlBody)
	msg := strings.Join(msgParts, "\r\n")

	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)

	var auth smtp.Auth
	if s.Username != "" {
		auth = smtp.PlainAuth("", s.Username, s.Password, s.Host)
	}

	// 端口 465 使用隐式 TLS；其他端口使用 STARTTLS（smtp.SendMail 自动处理）
	if s.Port == 465 {
		return s.sendImplicitTLS(addr, auth, from, []string{to}, []byte(msg))
	}
	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
}

// sendImplicitTLS 使用隐式 TLS 连接发送（适用于端口 465）
func (s *SMTPSender) sendImplicitTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	host := strings.Split(addr, ":")[0]

	conn, err := tls.Dial("tcp", addr, &tls.Config{
		ServerName: host,
	})
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Quit()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	if err := client.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return err
		}
	}

	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	return w.Close()
}

// IsConfigured 检查 SMTP 是否已配置
func (s *SMTPSender) IsConfigured() bool {
	return s.Host != "" && s.Port > 0
}

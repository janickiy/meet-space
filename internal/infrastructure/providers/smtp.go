// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package providers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	d "github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/domain/jobs"
)

// SMTPConfig определяет доверенный сервер доставки; незашифрованный текст SMTP и отключенная
// проверка сертификатов не поддерживаются.
type SMTPConfig struct {
	Host, Username, Password, From, TLSMode string
	Port                                    int
	Timeout                                 time.Duration
}

// SMTPEmail доставляет электронную почту MIME через TLS с проверкой по имени хоста и AUTH
// PLAIN. При повторных попытках сохраняется заголовок Message-ID, но SMTP не гарантирует
// однократную доставку.
type SMTPEmail struct {
	config    SMTPConfig
	from      *mail.Address
	tlsConfig *tls.Config
}

// NewSMTPEmail формирует электронное письмо SMTP с учетом зависимостей и ограничений,
// необходимых для интеграции с внешними системами.
//
// @Parameters:
//   - cfg: Ограничения конфигурации и обработки, контролируемые оператором.
//
// @returns:
//   - result1: Сформированное или загруженное электронное письмо SMTP.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NewSMTPEmail(cfg SMTPConfig) (*SMTPEmail, error) {
	if !smtpHostValid(cfg.Host) || cfg.Port < 1 || cfg.Port > 65535 {
		return nil, errors.New("SMTP requires a valid hostname and port")
	}
	if cfg.TLSMode != "tls" && cfg.TLSMode != "starttls" {
		return nil, errors.New("SMTP requires tls or starttls")
	}
	if cfg.Username == "" || len(cfg.Username) > 512 || strings.ContainsAny(cfg.Username, "\r\n\x00") || cfg.Password == "" || len(cfg.Password) > 4096 || strings.ContainsRune(cfg.Password, 0) {
		return nil, errors.New("SMTP credentials are required")
	}
	from, err := smtpAddress(cfg.From)
	if err != nil || len(cfg.From) > 512 {
		return nil, errors.New("SMTP requires one valid sender address")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.Timeout < time.Second || cfg.Timeout > time.Minute {
		return nil, errors.New("SMTP timeout must be within 1s..1m")
	}
	return &SMTPEmail{config: cfg, from: from, tlsConfig: &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}}, nil
}

// Send доставляет ограниченное электронное письмо со стабильным идентификатором сообщения;
// настроенное транспортное средство определяет возможность дедупликации при повторных попытках
// доставки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - message: Ограниченное содержимое электронного письма, получатель и стабильная
//     идентификация доставки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (a *SMTPEmail) Send(ctx context.Context, message d.EmailMessage) error {
	to, raw, err := a.message(message)
	if err != nil {
		return jobs.Error{Code: "smtp_input"}
	}
	ctx, cancel := context.WithTimeout(ctx, a.config.Timeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(a.config.Host, strconv.Itoa(a.config.Port)))
	if err != nil {
		return smtpError("smtp_unavailable", err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return smtpError("smtp_unavailable", err)
	}
	var transport net.Conn = conn
	if a.config.TLSMode == "tls" {
		secured := tls.Client(conn, a.tlsConfig.Clone())
		if err := secured.HandshakeContext(ctx); err != nil {
			return smtpTLSError(err)
		}
		transport = secured
	}
	client, err := smtp.NewClient(transport, a.config.Host)
	if err != nil {
		return smtpError("smtp_unavailable", err)
	}
	defer client.Close()
	if err := client.Hello("localhost"); err != nil {
		return smtpError("smtp_unavailable", err)
	}
	if a.config.TLSMode == "starttls" {
		if supported, _ := client.Extension("STARTTLS"); !supported {
			return jobs.Error{Code: "smtp_tls_required"}
		}
		if err := client.StartTLS(a.tlsConfig.Clone()); err != nil {
			return smtpTLSError(err)
		}
	}
	supported, mechanisms := client.Extension("AUTH")
	plain := false
	for _, mechanism := range strings.Fields(mechanisms) {
		plain = plain || strings.EqualFold(mechanism, "PLAIN")
	}
	if !supported || !plain {
		return jobs.Error{Code: "smtp_auth_unsupported"}
	}
	if err := client.Auth(smtp.PlainAuth("", a.config.Username, a.config.Password, a.config.Host)); err != nil {
		return smtpError("smtp_auth", err)
	}
	if err := client.Mail(a.from.Address); err != nil {
		return smtpError("smtp_sender", err)
	}
	if err := client.Rcpt(to.Address); err != nil {
		return smtpError("smtp_recipient", err)
	}
	writer, err := client.Data()
	if err != nil {
		return smtpError("smtp_data", err)
	}
	if _, err := writer.Write(raw); err != nil {
		return smtpError("smtp_data", err)
	}
	if err := writer.Close(); err != nil {
		return smtpError("smtp_data", err)
	}
	_ = client.Quit()
	return nil
}

// message формирует составное электронное письмо с ограничениями, не допуская внедрения
// заголовков.
//
// @Parameters:
//   - message: Ограниченное содержимое электронного письма, получатель и стабильная
//     идентификация доставки.
//
// @returns:
//   - result1: Сформированный или загруженный адрес.
//   - result2: Собранные байтовые значения.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (a *SMTPEmail) message(message d.EmailMessage) (*mail.Address, []byte, error) {
	to, err := smtpAddress(message.To)
	if err != nil || len(message.To) > 512 || message.IdempotencyKey == "" || len(message.IdempotencyKey) > 512 || !utf8.ValidString(message.Subject) || len(message.Subject) > 2048 || strings.ContainsAny(message.Subject, "\r\n\x00") || len(message.Text)+len(message.HTML) > 1<<20 || !utf8.ValidString(message.Text) || !utf8.ValidString(message.HTML) {
		return nil, nil, errors.New("invalid SMTP message")
	}
	subject, err := smtpSubject(message.Subject)
	if err != nil {
		return nil, nil, err
	}
	from, err := smtpHeaderValue(a.from.String(), len("From: "))
	if err != nil {
		return nil, nil, err
	}
	recipient, err := smtpHeaderValue(to.String(), len("To: "))
	if err != nil {
		return nil, nil, err
	}
	sum := sha256.Sum256([]byte(message.IdempotencyKey))
	key := hex.EncodeToString(sum[:])
	domain := a.from.Address[strings.LastIndexByte(a.from.Address, '@')+1:]
	var raw bytes.Buffer
	body := multipart.NewWriter(&raw)
	if err := body.SetBoundary("meetrix-" + key[:48]); err != nil {
		return nil, nil, err
	}
	fmt.Fprintf(&raw, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%s@%s>\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%q\r\n\r\n",
		from, recipient, subject, time.Now().UTC().Format(time.RFC1123Z), key, domain, body.Boundary())
	for _, part := range []struct{ kind, content string }{{"text/plain", message.Text}, {"text/html", message.HTML}} {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Type", part.kind+"; charset=UTF-8")
		header.Set("Content-Transfer-Encoding", "quoted-printable")
		writer, err := body.CreatePart(header)
		if err != nil {
			return nil, nil, err
		}
		encoded := quotedprintable.NewWriter(writer)
		if _, err := encoded.Write([]byte(part.content)); err != nil {
			return nil, nil, err
		}
		if err := encoded.Close(); err != nil {
			return nil, nil, err
		}
	}
	if err := body.Close(); err != nil {
		return nil, nil, err
	}
	return to, raw.Bytes(), nil
}

// smtpSubject кодирует тему письма с ограничениями, не допуская внедрения заголовков.
//
// @Parameters:
//   - subject: Валидированная тема письма.
//
// @returns:
//   - result1: Валидированная тема письма, закодированная с помощью MIME.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func smtpSubject(subject string) (string, error) {
	return smtpHeaderValue(mime.QEncoding.Encode("UTF-8", subject), len("Subject: "))
}

// smtpHeaderValue проверяет значение заголовка электронного письма перед его включением в
// сообщение SMTP.
//
// @Parameters:
//   - value: Значение для проверки, нормализации или преобразования.
//   - prefixLength: Максимальная длина безопасного диагностического префикса.
//
// @returns:
//   - result1: Проверенный заголовок электронного письма без внедрения управляющих
//     символов.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func smtpHeaderValue(value string, prefixLength int) (string, error) {
	words := strings.Split(value, " ")
	var header strings.Builder
	line := prefixLength
	for index, word := range words {
		if len(word) > 988 {
			return "", errors.New("SMTP subject word exceeds header limit")
		}
		if index > 0 {
			if line+len(word)+1 > 78 {
				header.WriteString("\r\n\t")
				line = 1
			} else {
				header.WriteByte(' ')
				line++
			}
		}
		header.WriteString(word)
		line += len(word)
	}
	return header.String(), nil
}

// smtpAddress проверяет и форматирует адрес электронной почты для заголовков SMTP и команд
// конверта.
//
// @Parameters:
//   - raw: Недоверенное сериализованное значение для декодирования или проверки.
//
// @returns:
//   - result1: Сформированный или загруженный адрес.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func smtpAddress(raw string) (*mail.Address, error) {
	if strings.ContainsAny(raw, "\r\n\x00") {
		return nil, errors.New("invalid email address")
	}
	address, err := mail.ParseAddress(raw)
	if err != nil || !strings.Contains(address.Address, "@") {
		return nil, errors.New("invalid email address")
	}
	for _, char := range address.Address {
		if char <= 32 || char >= 127 {
			return nil, errors.New("SMTP requires an ASCII mailbox address")
		}
	}
	return address, nil
}

// smtpHostValid проверяет, можно ли безопасно использовать настроенный хост SMTP в качестве
// имени сервера.
//
// @Parameters:
//   - host: Удаленный хост, настроенный оператором.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func smtpHostValid(host string) bool {
	if host == "" || len(host) > 253 || strings.ContainsAny(host, "/\\\r\n\t @?#") || strings.TrimSpace(host) != host {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-') {
				return false
			}
		}
	}
	return true
}

// smtpError классифицирует ошибку SMTP, не раскрывая ответ сервера или учетные данные.
//
// @Parameters:
//   - code: Ограниченный публичный статус или код ошибки.
//   - err: Ошибка классификации, инкапсуляции или безопасного оповещения.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func smtpError(code string, err error) error {
	var response *textproto.Error
	if errors.As(err, &response) {
		return jobs.Error{Code: code, Retryable: response.Code >= 400 && response.Code < 500}
	}
	return jobs.Error{Code: code, Retryable: true}
}

// smtpTLSError классифицирует ошибку согласования TLS, не раскрывая конфиденциальные
// диагностические данные о соединении.
//
// @Parameters:
//   - err: Ошибка классификации, инкапсуляции или безопасного оповещения.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func smtpTLSError(err error) error {
	var network net.Error
	if errors.As(err, &network) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return jobs.Error{Code: "smtp_tls", Retryable: true}
	}
	return jobs.Error{Code: "smtp_tls"}
}

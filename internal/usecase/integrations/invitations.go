// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package integrations

import (
	"bytes"
	"context"
	"html/template"
	"net/url"
	"strings"
	texttemplate "text/template"
	"time"

	"github.com/janickiy/meet-space/internal/domain/conferences"
	d "github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/domain/jobs"
)

type InvitationDelivery struct {
	Invitation conferences.Invitation
	Conference ConferenceSnapshot
	Organizer  string
	Allowed    bool
}

type invitationDeliveryRepository interface {
	// InvitationDelivery загружает текущие данные о доставке приглашения для арендованного
	// задания.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Зарезервированное фоновое задание и токен его генерации или владельца.
	//
	// @returns:
	//   - result1: В результате доставки приглашения.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	InvitationDelivery(context.Context, jobs.Job) (InvitationDelivery, error)
	// CompleteInvitation запись о доставке приглашения сохраняется только для текущего
	// идентификатора задания.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Зарезервированное фоновое задание и токен его генерации или владельца.
	//   - arg3: Состояние жизненного цикла или статус ответа на запись.
	//   - arg4: Ограниченный публичный статус или код ошибки.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	CompleteInvitation(context.Context, jobs.Job, string, string) error
}

// deliverInvitation доставляет текущее задание по приглашению, используя настройки получателя и
// данные об аренде.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) deliverInvitation(ctx context.Context, job jobs.Job) error {
	repo, ok := s.repo.(invitationDeliveryRepository)
	if !ok {
		return jobs.Error{Code: "invitation_repository"}
	}
	delivery, err := repo.InvitationDelivery(ctx, job)
	if err != nil {
		return err
	}
	if !delivery.Allowed {
		if err := repo.CompleteInvitation(ctx, job, "skipped", ""); err != nil {
			return err
		}
		return jobs.ErrSkip
	}
	if s.adapters.Capabilities.Email == "noop" || s.adapters.Email == nil {
		return jobs.Error{Code: "invitation_email_unconfigured", Retryable: true}
	}
	link := strings.TrimRight(s.options.PublicURL, "/") + "/i/" + url.PathEscape(delivery.Conference.InviteCode)
	message, err := RenderInvitationEmail(job.DedupKey, delivery.Invitation.Email, delivery.Conference.Title, delivery.Organizer, delivery.Conference.ScheduledAt, link)
	if err != nil {
		return err
	}
	op, cancel := context.WithTimeout(ctx, s.options.ProviderTimeout)
	defer cancel()
	if err := s.adapters.Email.Send(op, message); err != nil {
		return err
	}
	return repo.CompleteInvitation(ctx, job, "sent", "")
}

// deliverInvitationReminder sends a scheduled follow-up to the original invitee.
// ctx bounds delivery and job identifies the invitation and schedule version.
// The job runner records completion and retries independently of the original invitation.
func (s *Service) deliverInvitationReminder(ctx context.Context, job jobs.Job) error {
	repo, ok := s.repo.(invitationDeliveryRepository)
	if !ok {
		return jobs.Error{Code: "invitation_repository"}
	}
	delivery, err := repo.InvitationDelivery(ctx, job)
	if err != nil {
		return err
	}
	if !delivery.Allowed {
		return jobs.ErrSkip
	}
	if s.adapters.Capabilities.Email == "noop" || s.adapters.Email == nil {
		return jobs.Error{Code: "invitation_email_unconfigured", Retryable: true}
	}
	link := strings.TrimRight(s.options.PublicURL, "/") + "/i/" + url.PathEscape(delivery.Conference.InviteCode)
	message, err := RenderInvitationReminderEmail(job.DedupKey, delivery.Invitation.Email, delivery.Conference.Title, delivery.Organizer, delivery.Conference.ScheduledAt, link)
	if err != nil {
		return err
	}
	op, cancel := context.WithTimeout(ctx, s.options.ProviderTimeout)
	defer cancel()
	return s.adapters.Email.Send(op, message)
}

var invitationHTML = template.Must(template.New("meeting-invitation-v1").Parse(`<h1>Приглашение на встречу</h1><p>Вас пригласили на встречу «{{.Title}}».</p><p>Организатор: {{.Organizer}}</p>{{if .Scheduled}}<p>Время встречи: {{.Scheduled}}</p>{{end}}<p><a href="{{.URL}}">Присоединиться к встрече</a></p><p>Перейдите по ссылке и укажите своё имя. Для подключения регистрация не требуется.</p>`))
var invitationText = texttemplate.Must(texttemplate.New("meeting-invitation-v1").Parse("Приглашение на встречу\nВас пригласили на встречу «{{.Title}}».\nОрганизатор: {{.Organizer}}\n{{if .Scheduled}}Время встречи: {{.Scheduled}}\n{{end}}{{.URL}}\nПерейдите по ссылке и укажите своё имя. Для подключения регистрация не требуется.\n"))
var invitationReminderHTML = template.Must(template.New("meeting-invitation-reminder-v1").Parse(`<h1>Напоминание о встрече</h1><p>Скоро состоится встреча «{{.Title}}», на которую вас пригласили.</p><p>Организатор: {{.Organizer}}</p>{{if .Scheduled}}<p>Начало встречи: {{.Scheduled}}</p>{{end}}<p><a href="{{.URL}}">Присоединиться к встрече</a></p><p>Сохраните ссылку, чтобы подключиться вовремя. Для подключения регистрация не требуется.</p>`))
var invitationReminderText = texttemplate.Must(texttemplate.New("meeting-invitation-reminder-v1").Parse("Напоминание о встрече\nСкоро состоится встреча «{{.Title}}», на которую вас пригласили.\nОрганизатор: {{.Organizer}}\n{{if .Scheduled}}Начало встречи: {{.Scheduled}}\n{{end}}{{.URL}}\nСохраните ссылку, чтобы подключиться вовремя. Для подключения регистрация не требуется.\n"))

// RenderInvitationReminderEmail renders escaped HTML and plain text for one invitee.
// key is the retry-stable delivery ID, to is the original recipient, title and organizer
// are public meeting details, scheduled is the UTC start time, and link is the trusted
// guest-capable invitation URL. The returned message contains no other recipients.
func RenderInvitationReminderEmail(key, to, title, organizer string, scheduled *time.Time, link string) (d.EmailMessage, error) {
	data := struct{ Title, Organizer, Scheduled, URL string }{Title: title, Organizer: organizer, URL: link}
	if scheduled != nil {
		data.Scheduled = scheduled.UTC().Format("02.01.2006 15:04 MST")
	}
	var html, text bytes.Buffer
	if err := invitationReminderHTML.Execute(&html, data); err != nil {
		return d.EmailMessage{}, err
	}
	if err := invitationReminderText.Execute(&text, data); err != nil {
		return d.EmailMessage{}, err
	}
	return d.EmailMessage{IdempotencyKey: key, To: to, Subject: "Напоминание о встрече", HTML: html.String(), Text: text.String()}, nil
}

// RenderInvitationEmail использует явный часовой пояс и экранированные значения
// организатора/заголовка.
//
// @Parameters:
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//   - to: Конечное состояние или получатель операции.
//   - title: Подтвержденное название конференции или уведомления.
//   - organizer: Публичное имя организатора, указанное в приглашении.
//   - scheduled: Необязательное время начала запланированной конференции.
//   - link: Доверенная ссылка на внешний интерфейс, созданная приложением.
//
// @returns:
//   - result1: Результирующее электронное письмо.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func RenderInvitationEmail(key, to, title, organizer string, scheduled *time.Time, link string) (d.EmailMessage, error) {
	data := struct{ Title, Organizer, Scheduled, URL string }{Title: title, Organizer: organizer, URL: link}
	if scheduled != nil {
		data.Scheduled = scheduled.UTC().Format("02.01.2006 15:04 MST")
	}
	var html, text bytes.Buffer
	if err := invitationHTML.Execute(&html, data); err != nil {
		return d.EmailMessage{}, err
	}
	if err := invitationText.Execute(&text, data); err != nil {
		return d.EmailMessage{}, err
	}
	return d.EmailMessage{IdempotencyKey: key, To: to, Subject: "Приглашение на встречу", HTML: html.String(), Text: text.String()}, nil
}

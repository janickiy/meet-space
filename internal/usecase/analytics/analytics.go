// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

// Аналитика пакета формирует агрегированные данные о присутствии и запускает фоновый пересчет.
package analytics

import (
	"context"
	domain "github.com/janickiy/meet-space/internal/domain/analytics"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"github.com/janickiy/meet-space/internal/operations"
	"sort"
	"time"
)

// Repository — репозиторий предоставляет наблюдаемые интервалы и атомарно публикует результаты только для
// текущего задания.
type Repository interface {
	// Source загружает сохраненные входные данные, необходимые для обработки технических
	// агрегированных данных конференции.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Зарезервированное фоновое задание и токен его генерации или владельца.
	//
	// @returns:
	//   - result1: Временная метка или граница допустимого срока действия, связанная с
	//   результатом.
	//   - result2: Временная метка или граница допустимого срока действия, связанная с
	//   результатом.
	//   - result3: Собранные значения интервалов.
	//   - result4: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Source(context.Context, jobs.Job) (time.Time, time.Time, []domain.Interval, error)
	// Save сохраняет результаты анализа, проверяя текущий идентификатор задания или ресурса.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Зарезервированное фоновое задание и токен его генерации или владельца.
	//   - arg3: Значение для проверки, нормализации или преобразования.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Save(context.Context, jobs.Job, domain.Conference) error
	// Read считывает состояние или поток аналитических данных, необходимые вызывающей стороне.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//
	// @returns:
	//   - result1: Результирующая конференция.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Read(context.Context, string, string) (domain.Conference, error)
	// Tick — тик выполняет одну ограниченную итерацию обслуживания или доставки после получения
	// доступного сохраненного состояния.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Tick(context.Context) error
}

// Service — сервис не зависит от распознавания речи; сбои при расшифровке не влияют на показатели
// технической конференции.
type Service struct {
	Repo    Repository
	Enabled bool
}

// Aggregate — агрегатор вычисляет агрегированные технические показатели конференции на основе сохраненных
// исходных данных.
//
// @Parameters:
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - start: Начало временного интервала в единицах измерения, определенных сигнатурой.
//   - end: Конец временного интервала в единицах измерения, определенных сигнатурой.
//   - intervals: Упорядоченные интервалы активности участников.
//
// @returns:
//   - result1: Результирующая конференция.
func Aggregate(conferenceID string, start, end time.Time, intervals []domain.Interval) domain.Conference {
	result := domain.Conference{ConferenceID: conferenceID, Enabled: true, ApproximateSpeaking: true, Timeline: []domain.Point{}, Participants: []domain.Participant{}, DurationMS: max(int64(0), end.Sub(start).Milliseconds())}
	intervalsByParticipant := map[string][]domain.Interval{}
	for _, interval := range intervals {
		if interval.Start.Before(start) {
			interval.Start = start
		}
		if interval.End.After(end) {
			interval.End = end
		}
		if interval.End.After(interval.Start) {
			intervalsByParticipant[interval.ParticipantID] = append(intervalsByParticipant[interval.ParticipantID], interval)
		}
	}
	changes := map[int64]int{}
	participantIDs := make([]string, 0, len(intervalsByParticipant))
	for participantID := range intervalsByParticipant {
		participantIDs = append(participantIDs, participantID)
	}
	sort.Strings(participantIDs)
	for _, participantID := range participantIDs {
		participantIntervals := intervalsByParticipant[participantID]
		sort.Slice(participantIntervals, func(i, j int) bool { return participantIntervals[i].Start.Before(participantIntervals[j].Start) })
		var participationMS int64
		merged := []domain.Interval{}
		for _, interval := range participantIntervals {
			n := len(merged)
			if n == 0 || interval.Start.After(merged[n-1].End) {
				merged = append(merged, interval)
				continue
			}
			if interval.End.After(merged[n-1].End) {
				merged[n-1].End = interval.End
			}
		}
		for _, interval := range merged {
			participationMS += interval.End.Sub(interval.Start).Milliseconds()
			changes[interval.Start.Sub(start).Milliseconds()]++
			changes[interval.End.Sub(start).Milliseconds()]--
		}
		result.Participants = append(result.Participants, domain.Participant{ParticipantID: participantID, ParticipationMS: participationMS})
	}
	result.ParticipantCount = len(result.Participants)
	changeTimes := make([]int64, 0, len(changes))
	for at := range changes {
		changeTimes = append(changeTimes, at)
	}
	sort.Slice(changeTimes, func(i, j int) bool { return changeTimes[i] < changeTimes[j] })
	bucketMS := max(int64(1), (result.DurationMS+598)/599)
	count := 0
	for _, at := range changeTimes {
		count += changes[at]
		point := domain.Point{AtMS: at, Count: max(0, count)}
		n := len(result.Timeline)
		if n > 0 && result.Timeline[n-1].AtMS/bucketMS == at/bucketMS {
			result.Timeline[n-1] = point
		} else {
			result.Timeline = append(result.Timeline, point)
		}
	}
	return result
}

// Handle — обработчик отправляет проверенную аналитическую операцию обработчику приложения.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Handle(ctx context.Context, job jobs.Job) error {
	if !s.Enabled {
		return jobs.ErrSkip
	}
	started := time.Now()
	defer func() { operations.Observe("analytics", time.Since(started).Seconds()) }()
	start, end, rows, err := s.Repo.Source(ctx, job)
	if err != nil {
		return err
	}
	return s.Repo.Save(ctx, job, Aggregate(job.ConferenceID, start, end, rows))
}

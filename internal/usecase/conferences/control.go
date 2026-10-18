package conferences

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/janickiy/meet-space/internal/domain/realtime"
)

// ControlRepository определяет заменяемый контракт зависимостей для жизненного цикла
// конференции и членства участников.
type ControlRepository interface {
	// Moderate применяет действие по модерации участников после проверки роли субъекта и
	// целевых ограничений.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Идентификатор участника конференции.
	//   - arg5: Валидированный запрос приложения или входящие параметры операции.
	//
	// @returns:
	//   - result1: Полученный участник.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Moderate(context.Context, string, string, string, domain.ModerationRequest) (domain.Participant, error)
	// UpdateMediaState сохраняет заявленное участником состояние источника мультимедиа.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg4: Текущее состояние или обновлённое валидированное состояние.
	//
	// @returns:
	//   - result1: Полученный участник.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	UpdateMediaState(context.Context, string, string, domain.MediaState) (domain.Participant, error)
	// ReconcileParticipants возвращает участников, чьи сохранённые медиа-ограничения требуют
	// согласования.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Граница, после которой выбираются результаты или состояние.
	//   - arg3: Максимальное количество результатов или операций в этом пакете.
	//
	// @returns:
	//   - result1: Собранные данные об участниках.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	ReconcileParticipants(context.Context, string, int) ([]domain.Participant, error)
}

// PolicyController определяет заменяемый контракт зависимостей для жизненного цикла конференции
// и членства участников.
type PolicyController interface {
	// SetParticipantPolicy обновляет политику участников для жизненного цикла конференции и
	// членства участников.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	//   - arg3: Идентификатор участника конференции.
	//   - arg4: Эффективная политика в отношении медиаданных на стороне сервера.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	SetParticipantPolicy(context.Context, string, string, media.ParticipantPolicy) error
}

// ControlEvents определяет заменяемый контракт зависимостей для жизненного цикла конференции и
// членства участников.
type ControlEvents interface {
	// Функция Broadcast публикует доверенное событие только для тех участников конференции или
	// беседы, которые в данный момент имеют к ней доступ.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Доверенное событие для публикации или обработки.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Broadcast(context.Context, realtime.Envelope) error
	// ConferenceChanged уведомляет подключенные сеансы или наблюдателей об изменении состояния
	// конференции.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
	ConferenceChanged(context.Context, string)
}

// ControlService координирует постоянную модерацию с принудительным применением правил к
// медиафайлам в реальном времени.
type ControlService struct {
	repo   ControlRepository
	media  PolicyController
	events ControlEvents
}

// NewControlService создаёт службу управления с зависимостями и ограничениями, необходимыми для
// жизненного цикла конференции и членства участников.
//
// @Parameters:
//   - repo: Зависимость персистентности, используемая операцией приложения.
//   - controller: Политика в отношении медиаданных и контроллер команд.
//   - events: Место назначения для зафиксированных событий жизненного цикла или чатов.
//
// @returns:
//   - result1: Созданная или загруженная служба управления.
func NewControlService(repo ControlRepository, controller PolicyController, events ControlEvents) *ControlService {
	return &ControlService{repo: repo, media: controller, events: events}
}

// Moderate применяет действие по модерации участников после проверки роли субъекта и целевых
// ограничений.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - participantID: Идентификатор участника конференции.
//   - request: Валидированный запрос приложения или входящие параметры операции.
//
// @returns:
//   - result1: Полученное представление участника.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *ControlService) Moderate(ctx context.Context, userID, conferenceID, participantID string, request domain.ModerationRequest) (domain.ParticipantView, error) {
	p, err := s.repo.Moderate(ctx, conferenceID, userID, participantID, request)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	if s.events != nil {
		kind := "participant.media.updated"
		if request.Action == "role" {
			kind = "participant.role.updated"
		}
		if request.Action == "kick" {
			kind = "participant.kicked"
		}
		_ = s.events.Broadcast(ctx, realtime.Event(kind, conferenceID, map[string]any{"participant": p.View()}))
		s.events.ConferenceChanged(ctx, conferenceID)
	}
	slog.Info("conference moderation", "conference_id", conferenceID, "actor_user_id", userID, "participant_id", participantID, "action", request.Action, "policy_version", p.MediaPolicyVersion)
	if s.media != nil {
		if err = s.media.SetParticipantPolicy(ctx, conferenceID, participantID, media.PolicyForParticipant(p)); err != nil {
			return p.View(), apperrors.New(apperrors.ErrUnavailable, "moderation saved; media enforcement is retrying")
		}
	}
	return p.View(), nil
}

// UpdateMediaState сохраняет заявленное участником состояние источника мультимедиа.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - userID: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - conferenceID: Идентификатор конференции, ограничивающий область действия операции.
//   - state: Текущее состояние или обновлённое валидированное состояние.
//
// @returns:
//   - result1: Полученное представление участника.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *ControlService) UpdateMediaState(ctx context.Context, userID, conferenceID string, state domain.MediaState) (domain.ParticipantView, error) {
	p, err := s.repo.UpdateMediaState(ctx, conferenceID, userID, state)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	if s.events != nil {
		_ = s.events.Broadcast(ctx, realtime.Event("participant.media.updated", conferenceID, map[string]any{"participant": p.View()}))
	}
	return p.View(), nil
}

// Disconnected обрабатывает закрытие физического соединения и освобождает только те ресурсы,
// которые были связаны с этим соединением.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - session: Постоянное или физическое состояние сессии, определяемое ее типом.
func (s *ControlService) Disconnected(ctx context.Context, session realtime.Session) {
	repo, ok := s.repo.(interface {
		// ClearDisconnectedMedia очищает объявленное медиа-состояние, если у участника больше
		// нет активного авторизованного сеанса.
		//
		// @Parameters:
		//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
		//   операции.
		//   - arg2: Идентификатор конференции, ограничивающий область действия операции.
		//   - arg3: Идентификатор участника конференции.
		//
		// @returns:
		//   - result1: Полученный участник.
		//   - result2: Указывает, выполняется ли задокументированное условие операции и
		//   произошло ли успешное изменение ее условного состояния.
		//   - result3: nil — в случае успеха; в противном случае — ошибка валидации,
		//   доступа, передачи данных или сохранения.
		ClearDisconnectedMedia(context.Context, string, string) (domain.Participant, bool, error)
	})
	if !ok {
		return
	}
	p, changed, err := repo.ClearDisconnectedMedia(ctx, session.ConferenceID, session.ParticipantID)
	if err == nil && changed && s.events != nil {
		_ = s.events.Broadcast(ctx, realtime.Event("participant.media.updated", session.ConferenceID, map[string]any{"participant": p.View()}))
	}
}

// Run управляет жизненным циклом конференции и членством участников до тех пор, пока контекст
// не будет отменен или пока не произойдет сбой в процессе обработки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
func (s *ControlService) Run(ctx context.Context) {
	if s.media == nil {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	after := uuid.Nil.String()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		query, cancel := context.WithTimeout(ctx, 3*time.Second)
		rows, err := s.repo.ReconcileParticipants(query, after, 64)
		cancel()
		if err != nil {
			continue
		}
		if len(rows) < 64 {
			after = uuid.Nil.String()
		} else {
			after = rows[len(rows)-1].ID
		}
		var workers sync.WaitGroup
		jobs := make(chan domain.Participant)
		for range min(8, len(rows)) {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for p := range jobs {
					op, done := context.WithTimeout(ctx, 2*time.Second)
					_ = s.media.SetParticipantPolicy(op, p.ConferenceID, p.ID, media.PolicyForParticipant(p))
					done()
				}
			}()
		}
		for _, p := range rows {
			select {
			case jobs <- p:
			case <-ctx.Done():
				close(jobs)
				workers.Wait()
				return
			}
		}
		close(jobs)
		workers.Wait()
	}
}

// Пакет jobs запускает устойчивую очередь с ограниченным набором независимых рабочих процессов.
package jobs

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"regexp"
	"sync"
	"time"

	domain "github.com/janickiy/meet-space/internal/domain/jobs"
)

// Handler — обработчик выполняет операцию; Fail фиксирует безопасный терминальный сбой в связанной
// сущности.
type Handler struct {
	Handle func(context.Context, domain.Job) error
	Fail   func(context.Context, domain.Job, string) error
}

// Pool — пул резервирует независимые параллельные процессы и время ожидания для одной фиксированной
// категории рабочих процессов.
type Pool struct {
	Kind        string
	Concurrency int
	MaxAttempts int // Zero сохраняет бюджет строк; значения от 1 до 10 дополнительно ограничивают количество
	// старых или инициированных триггерами заданий.
	Timeout time.Duration
	Handler Handler
}

// Runner опрашивает PostgreSQL, используя фиксированное количество горутин, без создания
// отдельного таймера для каждой конференции.
type Runner struct {
	drainMu  sync.Mutex
	draining bool
	active   int
	repo     domain.Repository
	pools    []Pool
	interval time.Duration
	observe  func(string, string, time.Duration)
}

// New создает компонент с зависимостями и ограничениями на обработку, необходимыми для фоновых
// заданий с ограниченным временем обработки.
//
// @Parameters:
//   - repo: Зависимость персистентности, используемая операцией приложения.
//   - pools: Независимые пулы для выполнения фоновых заданий с ограниченным временем обработки.
//   - interval: Интервал между итерациями ограниченного обслуживания.
//   - observe: Обратный вызов, фиксирующий результат выполнения ограниченного задания или
//     провайдера.
//
// @returns:
//   - result1: Созданный или загруженный раннер.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func New(repo domain.Repository, pools []Pool, interval time.Duration, observe func(string, string, time.Duration)) (*Runner, error) {
	if repo == nil || interval < 100*time.Millisecond || interval > time.Minute || len(pools) > 10 {
		return nil, errors.New("invalid job runner configuration")
	}
	seen := map[string]bool{}
	for _, pool := range pools {
		if pool.Kind == "" || seen[pool.Kind] || pool.Concurrency < 1 || pool.Concurrency > 16 || pool.MaxAttempts < 0 || pool.MaxAttempts > 10 || pool.Timeout < time.Second || pool.Timeout > 30*time.Minute || pool.Handler.Handle == nil {
			return nil, errors.New("invalid job pool configuration")
		}
		seen[pool.Kind] = true
	}
	return &Runner{repo: repo, pools: pools, interval: interval, observe: observe}, nil
}

// BeginDrain прекращает принимать новые задачи, пока не завершатся текущие операции.
func (r *Runner) BeginDrain() { r.drainMu.Lock(); r.draining = true; r.drainMu.Unlock() }

// Active возвращает количество операций, принадлежащих компоненту, которые необходимо завершить
// до завершения работы.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
func (r *Runner) Active() int { r.drainMu.Lock(); defer r.drainMu.Unlock(); return r.active }

// Run запускает арендованные фоновые задания до тех пор, пока контекст не будет отменен или
// пока не произойдет сбой при обработке.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
func (r *Runner) Run(ctx context.Context) {
	var workers sync.WaitGroup
	for _, pool := range r.pools {
		for range pool.Concurrency {
			workers.Go(func() { r.work(ctx, pool) })
		}
	}
	workers.Wait()
}

// work — обрабатывает одну категорию фоновых заданий с отдельной политикой аренды и выполнения.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - pool: Политика выполнения и обработчики для выбранной категории заданий.
func (r *Runner) work(ctx context.Context, pool Pool) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		r.drainMu.Lock()
		if r.draining {
			r.drainMu.Unlock()
			return
		}
		r.active++
		r.drainMu.Unlock()
		claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		job, found, err := r.repo.Claim(claimCtx, pool.Kind, pool.Timeout+30*time.Second)
		cancel()
		if err == nil && found {
			r.execute(ctx, pool, job)
			r.drainMu.Lock()
			r.active--
			r.drainMu.Unlock()
			continue
		}
		r.drainMu.Lock()
		r.active--
		r.drainMu.Unlock()
		if err != nil && ctx.Err() == nil {
			slog.Warn("product job queue unavailable", "event_type", "product.queue.failed", "kind", pool.Kind)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// execute запускает арендованное задание вне результирующей транзакции и применяет политику
// ограниченных повторных попыток.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - pool: Политика выполнения и обработчики для выбранной категории заданий.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
func (r *Runner) execute(ctx context.Context, pool Pool, job domain.Job) {
	start := time.Now()
	if pool.MaxAttempts > 0 && pool.MaxAttempts < job.MaxAttempts {
		job.MaxAttempts = pool.MaxAttempts
	}
	var err error
	if job.Attempts > job.MaxAttempts {
		err = &domain.Error{Code: "attempts_exhausted"}
	} else {
		callCtx, cancel := context.WithTimeout(ctx, pool.Timeout)
		err = callHandler(callCtx, pool.Handler.Handle, job)
		cancel()
	}
	state, code, retryable, wait := classify(err)
	var retryAt *time.Time
	if retryable && job.Attempts < job.MaxAttempts {
		delay := Backoff(job.Attempts, wait)
		next := time.Now().Add(delay)
		retryAt, state = &next, "queued"
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if state == "failed" && pool.Handler.Fail != nil {
		if failure := pool.Handler.Fail(finishCtx, job, code); failure != nil {
			slog.Warn("product failure state not saved", "event_type", "product.failure.pending", "kind", pool.Kind, "job_id", job.ID)
			return // Срок аренды истекает; при последующем запросе сохраняется терминальное состояние
			// без обращения к провайдеру.
		}
	}
	if finishErr := r.repo.Finish(finishCtx, job, state, code, retryAt); finishErr != nil {
		slog.Warn("product job completion deferred", "event_type", "product.completion.pending", "kind", pool.Kind, "job_id", job.ID)
		return
	}
	if r.observe != nil {
		r.observe(pool.Kind, state, time.Since(start))
	}
	slog.Info("product job completed", "event_type", "product.job.completed", "kind", pool.Kind, "state", state, "job_id", job.ID, "entity_id", job.EntityID, "conference_id", job.ConferenceID, "attempt", job.Attempts, "code", code)
}

var safeCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// callHandler вызывает обработчик арендованного задания и преобразует панику в контролируемый
// сбой рабочего процесса.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - handle: Обратный вызов, выполняющий арендованное фоновое задание.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//
// @returns:
//   - err: nil — в случае успеха; в противном случае — ошибка валидации, доступа, передачи
//     данных или сохранения.
func callHandler(ctx context.Context, handle func(context.Context, domain.Job) error, job domain.Job) (err error) {
	defer func() {
		if recover() != nil {
			err = domain.Error{Code: "handler_panic"}
		}
	}()
	return handle(ctx, job)
}

// classify преобразует ошибку обработчика в метаданные для безопасной повторной попытки без
// сохранения тела ответа от провайдера.
//
// @Parameters:
//   - err: Ошибка классификации, инкапсуляции или безопасного оповещения.
//
// @returns:
//   - result1: Терминальное состояние задания с ограничением или состояние задания для
//     повторной попытки.
//   - result2: Код безопасного сбоя без содержимого от провайдера или пользователя.
//   - result3: Возможность повторной попытки выполнения задания.
//   - result4: Запрошенная провайдером задержка между повторными попытками, до того как
//     исполнитель применит свои ограничения.
func classify(err error) (string, string, bool, time.Duration) {
	if err == nil {
		return "done", "", false, 0
	}
	if errors.Is(err, domain.ErrSkip) {
		return "skipped", "", false, 0
	}
	if errors.Is(err, domain.ErrLeaseLost) {
		return "skipped", "lease_lost", false, 0
	}
	var failure *domain.Error
	var valueFailure domain.Error
	if errors.As(err, &valueFailure) {
		failure = &valueFailure
	} else {
		_ = errors.As(err, &failure)
	}
	if failure != nil {
		code := failure.Code
		if !safeCode.MatchString(code) {
			code = "provider_failed"
		}
		return "failed", code, failure.Retryable, failure.RetryAfter
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "failed", "operation_timeout", true, 0
	}
	return "failed", "internal_unavailable", true, 0
}

// Backoff вычисляет ограниченную задержку между повторными попытками для фонового задания.
//
// @Parameters:
//   - attempt: Текущая повторная попытка.
//   - requested: Запрошенная задержка между повторными попытками, до того как она будет
//     ограничена в соответствии с поддерживаемой политикой.
//
// @returns:
//   - result1: Продолжительность или ограниченный интервал ожидания, вычисленный в ходе
//     операции.
func Backoff(attempt int, requested time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 10 {
		attempt = 10
	}
	base := time.Second * time.Duration(1<<uint(attempt))
	delay := base/2 + time.Duration(rand.Int64N(int64(base/2)+1))
	if requested > delay {
		delay = requested
	}
	if delay > time.Hour {
		delay = time.Hour
	}
	return delay
}

package sfu

import (
	"log/slog"

	"github.com/pion/logging"
)

// safePionLoggerFactory создает логгер Pion, который сохраняет только ограниченные метаданные о
// компонентах и уровнях.
type safePionLoggerFactory struct {
	logger *slog.Logger
}

// safePionLogger отбрасывает потенциально конфиденциальные данные Pion SDP, ICE, текст
// сообщения и аргументы форматирования.
type safePionLogger struct {
	logger    *slog.Logger
	component string
}

var _ logging.LoggerFactory = safePionLoggerFactory{}
var _ logging.LeveledLogger = safePionLogger{}

// NewLogger создает логгер с зависимостями и ограничениями, необходимыми для одноранговых узлов
// SFU и пересылки медиаданных.
//
// @Parameters:
//   - scope: Авторизованный диалог или область доступа к ресурсу.
//
// @returns:
//   - result1: Результирующий логгер с разбивкой по уровням.
func (f safePionLoggerFactory) NewLogger(scope string) logging.LeveledLogger {
	component := "other"
	switch scope {
	case "api", "pc", "ice", "dtls", "srtp", "sctp", "ortc", "mux", "datachannel",
		"nack_generator", "nack_responder", "receiver_interceptor", "sender_interceptor",
		"stats", "twcc_sender_interceptor":
		component = scope
	case "RTPReceiver":
		component = "rtp_receiver"
	case "RTPSender":
		component = "rtp_sender"
	case "DTLSTransport":
		component = "dtls_transport"
	}
	return safePionLogger{logger: f.logger, component: component}
}

// Trace записывает только безопасную классификацию логов Pion; текст сообщения и аргументы
// форматирования намеренно не отображаются.
//
// @Parameters:
//   - arg1: Исходное сообщение или формат Pion; намеренно игнорируется для защиты
//     конфиденциальных сигнальных данных.
func (l safePionLogger) Trace(string) {}

// Tracef записывает только безопасную классификацию логов Pion; текст сообщения и аргументы
// форматирования намеренно не отображаются.
//
// @Parameters:
//   - arg1: Исходное сообщение или формат Pion; намеренно игнорируется для защиты
//     конфиденциальных сигнальных данных.
//   - arg2: Исходные значения форматирования Pion; намеренно игнорируются для защиты
//     конфиденциальных сигнальных данных.
func (l safePionLogger) Tracef(string, ...any) {}

// Debug записывает только безопасную классификацию логов Pion; текст сообщения и аргументы
// форматирования намеренно не отображаются.
//
// @Parameters:
//   - arg1: Исходное сообщение или формат Pion; намеренно игнорируется для защиты
//     конфиденциальных сигнальных данных.
func (l safePionLogger) Debug(string) {}

// Debugf записывает только безопасную классификацию логов Pion; текст сообщения и аргументы
// форматирования намеренно не отображаются.
//
// @Parameters:
//   - arg1: Исходное сообщение или формат Pion; намеренно игнорируется для защиты
//     конфиденциальных сигнальных данных.
//   - arg2: Исходные значения форматирования Pion; намеренно игнорируются для защиты
//     конфиденциальных сигнальных данных.
func (l safePionLogger) Debugf(string, ...any) {}

// Info записывает только безопасную классификацию логов Pion; текст сообщения и аргументы
// форматирования намеренно не отображаются.
//
// @Parameters:
//   - arg1: Исходное сообщение или формат Pion; намеренно игнорируется для защиты
//     конфиденциальных сигнальных данных.
func (l safePionLogger) Info(string) {
	l.logger.Info("media transport event", "event_type", "pion_info", "component", l.component)
}

// Infof записывает только безопасную классификацию логов Pion; текст сообщения и аргументы
// форматирования намеренно не отображаются.
//
// @Parameters:
//   - arg1: Исходное сообщение или формат Pion; намеренно игнорируется для защиты
//     конфиденциальных сигнальных данных.
//   - arg2: Исходные значения форматирования Pion; намеренно игнорируются для защиты
//     конфиденциальных сигнальных данных.
func (l safePionLogger) Infof(string, ...any) { l.Info("") }

// Warn записывает только безопасную классификацию логов Pion; текст сообщения и аргументы
// форматирования намеренно не отображаются.
//
// @Parameters:
//   - arg1: Исходное сообщение или формат Pion; намеренно игнорируется для защиты
//     конфиденциальных сигнальных данных.
func (l safePionLogger) Warn(string) {
	l.logger.Warn("media transport event", "event_type", "pion_warning", "component", l.component)
}

// Warnf записывает только безопасную классификацию логов Pion; текст сообщения и аргументы
// форматирования намеренно не отображаются.
//
// @Parameters:
//   - arg1: Исходное сообщение или формат Pion; намеренно игнорируется для защиты
//     конфиденциальных сигнальных данных.
//   - arg2: Исходные значения форматирования Pion; намеренно игнорируются для защиты
//     конфиденциальных сигнальных данных.
func (l safePionLogger) Warnf(string, ...any) { l.Warn("") }

// Error записывает только безопасную классификацию логов Pion; текст сообщения и аргументы
// форматирования намеренно не отображаются.
//
// @Parameters:
//   - arg1: Исходное сообщение или формат Pion; намеренно игнорируется для защиты
//     конфиденциальных сигнальных данных.
func (l safePionLogger) Error(string) {
	l.logger.Error("media transport event", "event_type", "pion_error", "component", l.component)
}

// Errorf записывает только безопасную классификацию логов Pion; текст сообщения и аргументы
// форматирования намеренно не отображаются.
//
// @Parameters:
//   - arg1: Исходное сообщение или формат Pion; намеренно игнорируется для защиты
//     конфиденциальных сигнальных данных.
//   - arg2: Исходные значения форматирования Pion; намеренно игнорируются для защиты
//     конфиденциальных сигнальных данных.
func (l safePionLogger) Errorf(string, ...any) { l.Error("") }

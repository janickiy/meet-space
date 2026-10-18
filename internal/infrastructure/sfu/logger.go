package sfu

import (
	"log/slog"

	"github.com/pion/logging"
)

// safePionLoggerFactory builds a Pion logger that retains only bounded component and level
// metadata.
type safePionLoggerFactory struct {
	logger *slog.Logger
}

// safePionLogger discards potentially private Pion SDP, ICE, message text and formatting
// arguments.
type safePionLogger struct {
	logger    *slog.Logger
	component string
}

var _ logging.LoggerFactory = safePionLoggerFactory{}
var _ logging.LeveledLogger = safePionLogger{}

// NewLogger constructs the logger with the dependencies and limits required for SFU peers and
// media forwarding.
//
// Parameters:
//   - scope: Authorized conversation or resource scope.
//
// Returns:
//   - result 1: The resulting leveled logger.
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

// Trace records only the safe Pion log classification; message text and formatting arguments are
// deliberately not exposed.
//
// Parameters:
//   - argument 1: Original Pion message or format; deliberately ignored to protect private
//     signaling data.
func (l safePionLogger) Trace(string) {}

// Tracef records only the safe Pion log classification; message text and formatting arguments
// are deliberately not exposed.
//
// Parameters:
//   - argument 1: Original Pion message or format; deliberately ignored to protect private
//     signaling data.
//   - argument 2: Original Pion formatting values; deliberately ignored to protect private
//     signaling data.
func (l safePionLogger) Tracef(string, ...any) {}

// Debug records only the safe Pion log classification; message text and formatting arguments are
// deliberately not exposed.
//
// Parameters:
//   - argument 1: Original Pion message or format; deliberately ignored to protect private
//     signaling data.
func (l safePionLogger) Debug(string) {}

// Debugf records only the safe Pion log classification; message text and formatting arguments
// are deliberately not exposed.
//
// Parameters:
//   - argument 1: Original Pion message or format; deliberately ignored to protect private
//     signaling data.
//   - argument 2: Original Pion formatting values; deliberately ignored to protect private
//     signaling data.
func (l safePionLogger) Debugf(string, ...any) {}

// Info records only the safe Pion log classification; message text and formatting arguments are
// deliberately not exposed.
//
// Parameters:
//   - argument 1: Original Pion message or format; deliberately ignored to protect private
//     signaling data.
func (l safePionLogger) Info(string) {
	l.logger.Info("media transport event", "event_type", "pion_info", "component", l.component)
}

// Infof records only the safe Pion log classification; message text and formatting arguments are
// deliberately not exposed.
//
// Parameters:
//   - argument 1: Original Pion message or format; deliberately ignored to protect private
//     signaling data.
//   - argument 2: Original Pion formatting values; deliberately ignored to protect private
//     signaling data.
func (l safePionLogger) Infof(string, ...any) { l.Info("") }

// Warn records only the safe Pion log classification; message text and formatting arguments are
// deliberately not exposed.
//
// Parameters:
//   - argument 1: Original Pion message or format; deliberately ignored to protect private
//     signaling data.
func (l safePionLogger) Warn(string) {
	l.logger.Warn("media transport event", "event_type", "pion_warning", "component", l.component)
}

// Warnf records only the safe Pion log classification; message text and formatting arguments are
// deliberately not exposed.
//
// Parameters:
//   - argument 1: Original Pion message or format; deliberately ignored to protect private
//     signaling data.
//   - argument 2: Original Pion formatting values; deliberately ignored to protect private
//     signaling data.
func (l safePionLogger) Warnf(string, ...any) { l.Warn("") }

// Error records only the safe Pion log classification; message text and formatting arguments are
// deliberately not exposed.
//
// Parameters:
//   - argument 1: Original Pion message or format; deliberately ignored to protect private
//     signaling data.
func (l safePionLogger) Error(string) {
	l.logger.Error("media transport event", "event_type", "pion_error", "component", l.component)
}

// Errorf records only the safe Pion log classification; message text and formatting arguments
// are deliberately not exposed.
//
// Parameters:
//   - argument 1: Original Pion message or format; deliberately ignored to protect private
//     signaling data.
//   - argument 2: Original Pion formatting values; deliberately ignored to protect private
//     signaling data.
func (l safePionLogger) Errorf(string, ...any) { l.Error("") }

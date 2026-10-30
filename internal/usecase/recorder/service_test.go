package recorder

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/janickiy/meet-space/internal/domain/records"
)

type commandRecordRepository struct {
	record records.Record
	err    error
	calls  int
	id     string
}

// FindByUUID records the requested id and returns the configured read-only result.
func (r *commandRecordRepository) FindByUUID(_ context.Context, id string) (records.Record, error) {
	r.calls++
	r.id = id
	return r.record, r.err
}

type recordingCommandSpy struct {
	calls   int
	command records.Command
	record  records.Record
	err     error
}

// HandleCommand captures command and record for dispatch assertions.
func (s *recordingCommandSpy) HandleCommand(_ context.Context, command records.Command, record records.Record) error {
	s.calls++
	s.command, s.record = command, record
	return s.err
}

// sfuTestRecord returns a valid SFU recording fixture for the requested mode.
func sfuTestRecord(mode string) records.Record {
	conference := "0fbfa099-e4dc-4441-ad28-ec89a3c51f38"
	return records.Record{
		UUID: "a2401653-24bc-447e-a663-8b525d799088", ConferenceID: conference,
		PlatformConferenceID: &conference, Mode: mode, SourceType: "conference",
		TransportType: "sfu", Status: records.StatusStarting,
	}
}

// TestWorkerRejectsUnsupportedRecordings verifies that old queue commands cannot
// invoke processing or mutate the persisted recording, including terminal records.
func TestWorkerRejectsUnsupportedRecordings(t *testing.T) {
	for _, status := range []string{records.StatusStarting, records.StatusRecording, records.StatusReady, records.StatusFailed} {
		for _, operation := range []string{"record.start", "record.stop"} {
			t.Run(status+"/"+operation, func(t *testing.T) {
				record := sfuTestRecord("legacy")
				record.Status = status
				record.SourceType, record.TransportType, record.PlatformConferenceID = "browser", "webrtc", nil
				repo := &commandRecordRepository{record: record}
				processor := &recordingCommandSpy{}
				service := NewWorkerService(repo, processor)
				err := service.HandleCommand(context.Background(), records.Command{Type: operation, RecordID: record.UUID})
				if !errors.Is(err, records.ErrUnsupportedRecording) {
					t.Fatalf("expected unsupported recording, got %v", err)
				}
				if processor.calls != 0 || !reflect.DeepEqual(repo.record, record) {
					t.Fatal("unsupported recording was dispatched or mutated")
				}
			})
		}
	}
}

// TestWorkerDispatchesSFUCommands checks all supported recording modes and preserves
// command fields and processor errors while normalizing the record UUID.
func TestWorkerDispatchesSFUCommands(t *testing.T) {
	for _, mode := range []string{records.ModeComposite, records.ModeAudioOnly, records.ModeIndividualTracks, records.ModeScreenFocus} {
		for _, operation := range []string{"record.start", "record.stop"} {
			t.Run(mode+"/"+operation, func(t *testing.T) {
				record := sfuTestRecord(mode)
				repo := &commandRecordRepository{record: record}
				processorErr := errors.New("processor failure")
				processor := &recordingCommandSpy{err: processorErr}
				service := NewWorkerService(repo, processor)
				command := records.Command{Type: operation, RecordID: strings.ToUpper(record.UUID), Reason: "requested", SegmentDurationSec: 5}
				if err := service.HandleCommand(context.Background(), command); !errors.Is(err, processorErr) {
					t.Fatalf("expected processor error, got %v", err)
				}
				command.RecordID = record.UUID
				if processor.calls != 1 || repo.calls != 1 || repo.id != record.UUID || processor.command != command || !reflect.DeepEqual(processor.record, record) {
					t.Fatalf("incorrect SFU dispatch: repository=%+v processor=%+v", repo, processor)
				}
			})
		}
	}
}

// TestWorkerValidationPrecedesLookup ensures invalid commands never access storage.
func TestWorkerValidationPrecedesLookup(t *testing.T) {
	for _, command := range []records.Command{
		{Type: "record.start", RecordID: "../../unexpected"},
		{Type: "record.delete", RecordID: sfuTestRecord(records.ModeComposite).UUID},
	} {
		repo := &commandRecordRepository{}
		processor := &recordingCommandSpy{}
		if err := NewWorkerService(repo, processor).HandleCommand(context.Background(), command); err == nil {
			t.Fatal("invalid command was accepted")
		}
		if repo.calls != 0 || processor.calls != 0 {
			t.Fatal("invalid command reached dependencies")
		}
	}
}

// TestWorkerLookupFailureDoesNotDispatch preserves database errors without processing.
func TestWorkerLookupFailureDoesNotDispatch(t *testing.T) {
	lookupErr := errors.New("record not found")
	repo := &commandRecordRepository{err: lookupErr}
	processor := &recordingCommandSpy{}
	command := records.Command{Type: "record.start", RecordID: sfuTestRecord(records.ModeComposite).UUID}
	if err := NewWorkerService(repo, processor).HandleCommand(context.Background(), command); !errors.Is(err, lookupErr) || processor.calls != 0 {
		t.Fatalf("lookup failure was not preserved: %v", err)
	}
}

// TestWorkerRequiresProcessor returns an actionable error instead of silently
// accepting a valid recording when the SFU pipeline was not configured.
func TestWorkerRequiresProcessor(t *testing.T) {
	record := sfuTestRecord(records.ModeComposite)
	repo := &commandRecordRepository{record: record}
	err := NewWorkerService(repo, nil).HandleCommand(context.Background(), records.Command{Type: "record.start", RecordID: record.UUID})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected processor configuration error, got %v", err)
	}
}

// TestCompositeRejectsUnsupportedRecordings covers direct dispatch and the
// reconciliation launch path before either can claim a lease or create files.
func TestCompositeRejectsUnsupportedRecordings(t *testing.T) {
	service := &CompositeService{}
	record := sfuTestRecord(records.ModeComposite)
	record.TransportType = "webrtc"
	command := records.Command{Type: "record.start", RecordID: record.UUID}
	if err := service.HandleCommand(context.Background(), command, record); !errors.Is(err, records.ErrUnsupportedRecording) {
		t.Fatalf("direct dispatch accepted unsupported recording: %v", err)
	}
	if err := service.launch(context.Background(), record); !errors.Is(err, records.ErrUnsupportedRecording) {
		t.Fatalf("reconciliation accepted unsupported recording: %v", err)
	}
}

type recordingCardRepository struct{ details records.RecordDetails }

// FindDetailsByUUID returns existing metadata without migrating or rewriting it.
func (r recordingCardRepository) FindDetailsByUUID(context.Context, string) (records.RecordDetails, error) {
	return r.details, nil
}

// TestExistingRecordingArtifactsRemainReadable keeps reading independent from
// the stricter processing contract, including metadata from earlier releases.
func TestExistingRecordingArtifactsRemainReadable(t *testing.T) {
	record := sfuTestRecord(records.ModeComposite)
	record.Status, record.TransportType = records.StatusReady, ""
	files := []records.RecordFile{{FileName: "final.mp4", ObjectKey: "records/previous/final.mp4", FileType: records.FileTypeFinalMP4}}
	service := NewService(recordingCardRepository{details: records.RecordDetails{Record: record, Files: files}}, nil)
	card, err := service.ReadComposite(context.Background(), record.UUID)
	if err != nil || len(card.Files) != 1 || card.Files[0].ObjectKey != files[0].ObjectKey {
		t.Fatalf("existing artifact metadata is no longer readable: card=%+v error=%v", card, err)
	}
}

package postgres_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	recordingsapp "github.com/janickiy/meet-space/internal/app/recordings"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/records"
	pg "github.com/janickiy/meet-space/internal/infrastructure/postgres"
	transport "github.com/janickiy/meet-space/internal/transport/http"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	recordingsusecase "github.com/janickiy/meet-space/internal/usecase/recordings"
)

type recordingDeleteStorage func(context.Context, records.Record) error

func (remove recordingDeleteStorage) RemoveRecording(ctx context.Context, record records.Record) error {
	return remove(ctx, record)
}

type recordingDeleteTokens struct{}

func (recordingDeleteTokens) Verify(token string) (string, error) {
	if _, err := uuid.Parse(token); err != nil {
		return "", apperrors.ErrUnauthorized
	}
	return token, nil
}

// All mutations run inside searchDB's rollback-only transaction in an explicit
// loopback test database. Storage is a callback; real S3 deletion has its own test.
func TestRecordingDeletion(t *testing.T) {
	db := searchDB(t)
	ctx := context.Background()
	owner := searchUser(t, db, "Organizer")
	author := searchUser(t, db, "Recorder")
	viewer := searchUser(t, db, "Viewer")
	guest := searchUser(t, db, "Guest")
	outsider := searchUser(t, db, "Outsider")
	cid := uuid.NewString()
	now := time.Now().UTC()
	conferenceRepo := pg.NewConferenceRepository(db)
	_, err := conferenceRepo.Create(ctx,
		conferences.Conference{ID: cid, OwnerID: owner, Kind: "meeting", Title: "Deletion test", InviteCode: strings.ReplaceAll(uuid.NewString(), "-", ""), Status: conferences.Finished, StartedAt: &now, FinishedAt: &now},
		conferences.Participant{ID: uuid.NewString(), ConferenceID: cid, UserID: &owner, DisplayName: "Organizer", Role: conferences.Owner, Status: conferences.Left, AdmissionState: conferences.AdmissionAdmitted})
	searchMust(t, err)
	for _, id := range []string{author, viewer, guest} {
		searchMust(t, db.Create(&conferences.Participant{ID: uuid.NewString(), ConferenceID: cid, UserID: &id, DisplayName: "Participant", Role: conferences.ParticipantRole, Status: conferences.Left, AdmissionState: conferences.AdmissionAdmitted}).Error)
	}
	searchMust(t, db.Table("users").Where("id=?", guest).Update("guest_conference_id", cid).Error)
	repo := pg.NewConferenceRecordingRepository(db)
	create := func(status, requester string) records.Record {
		t.Helper()
		r := records.Record{UUID: uuid.NewString(), ConferenceID: cid, PlatformConferenceID: &cid, RequestedBy: &requester, Mode: records.ModeComposite, SourceType: "conference", TransportType: "sfu", QualityMode: "auto", Status: status, EndedAt: &now}
		searchMust(t, db.Create(&r).Error)
		searchMust(t, db.Create(&records.RecordFile{RecordID: r.ID, UUID: uuid.NewString(), FileType: records.FileTypeFinalMP4, Bucket: "test", ObjectKey: r.UUID + "/final.mp4", MimeType: "video/mp4"}).Error)
		searchMust(t, db.Create(&records.RecordSegment{RecordID: r.ID, SeqNo: 0, Status: "uploaded"}).Error)
		return r
	}
	removed := 0
	remove := func(_ context.Context, _ records.Record) error { removed++; return nil }
	r := create(records.StatusReady, author)
	for _, id := range []string{viewer, outsider, guest} {
		if _, err := repo.Delete(ctx, id, cid, r.UUID, remove); !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatalf("unauthorized delete by %s: %v", id, err)
		}
	}
	// Guests cannot delete even their own legacy requested_by record.
	guestRecord := create(records.StatusReady, guest)
	if _, err := repo.Delete(ctx, guest, cid, guestRecord.UUID, remove); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("guest deleted own recording: %v", err)
	}
	searchMust(t, db.Table("conference_participants").Where("conference_id=? AND user_id=?", cid, author).Updates(map[string]any{"status": "kicked", "admission_state": "kicked"}).Error)
	if _, err := repo.Delete(ctx, author, cid, r.UUID, remove); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("kicked author could delete: %v", err)
	}
	searchMust(t, db.Table("conference_participants").Where("conference_id=? AND user_id=?", cid, author).Updates(map[string]any{"status": "left", "admission_state": "admitted"}).Error)
	if _, err := repo.Delete(ctx, owner, uuid.NewString(), r.UUID, remove); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("wrong conference accepted: %v", err)
	}
	otherID := uuid.NewString()
	_, err = conferenceRepo.Create(ctx,
		conferences.Conference{ID: otherID, OwnerID: owner, Kind: "meeting", Title: "Other meeting", InviteCode: strings.ReplaceAll(uuid.NewString(), "-", ""), Status: conferences.Created},
		conferences.Participant{ID: uuid.NewString(), ConferenceID: otherID, UserID: &owner, DisplayName: "Organizer", Role: conferences.Owner, Status: conferences.Joined, JoinedAt: &now, AdmissionState: conferences.AdmissionAdmitted})
	searchMust(t, err)
	if _, err := repo.Delete(ctx, owner, otherID, r.UUID, remove); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("recording ID accepted in a different authorized conference: %v", err)
	}
	if removed != 0 {
		t.Fatal("storage was touched before access checks")
	}
	for _, status := range []string{records.StatusStarting, records.StatusRecording, records.StatusDegraded, records.StatusStopping, records.StatusFinalizing, records.StatusUploading} {
		active := create(status, author)
		if _, err := repo.Delete(ctx, owner, cid, active.UUID, remove); !errors.Is(err, apperrors.ErrConflict) {
			t.Fatalf("deleted active %s: %v", status, err)
		}
		searchMust(t, db.Model(&active).Update("status", records.StatusFailed).Error)
	}
	searchMust(t, db.Model(&r).Update("recorder_lease_until", now.Add(time.Hour)).Error)
	if _, err := repo.Delete(ctx, owner, cid, r.UUID, remove); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("deleted leased recording: %v", err)
	}
	searchMust(t, db.Model(&r).Update("recorder_lease_until", now.Add(-time.Minute)).Error)
	storageError := errors.New("storage temporarily unavailable")
	if _, err := repo.Delete(ctx, author, cid, r.UUID, func(context.Context, records.Record) error { return storageError }); !errors.Is(err, storageError) {
		t.Fatalf("storage error lost: %v", err)
	}
	_, err = repo.Accessible(ctx, owner, cid, r.UUID)
	searchMust(t, err)
	var count int64
	searchMust(t, db.Model(&records.RecordFile{}).Where("record_id=?", r.ID).Count(&count).Error)
	if count != 1 || removed != 0 {
		t.Fatal("failed deletion removed metadata or active recording reached storage")
	}
	_, err = repo.Delete(ctx, author, cid, r.UUID, remove)
	searchMust(t, err)
	_, err = repo.Delete(ctx, author, cid, r.UUID, remove)
	searchMust(t, err)
	if removed != 1 {
		t.Fatal("repeat deletion was not idempotent")
	}
	for _, model := range []any{&records.RecordFile{}, &records.RecordSegment{}} {
		searchMust(t, db.Model(model).Where("record_id=?", r.ID).Count(&count).Error)
		if count != 0 {
			t.Fatal("artifact metadata remains")
		}
	}
	if _, err := repo.Accessible(ctx, owner, cid, r.UUID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("deleted recording remains accessible: %v", err)
	}
	items, err := repo.List(ctx, owner, cid, 100, 0)
	searchMust(t, err)
	for _, item := range items {
		if item.UUID == r.UUID {
			t.Fatal("deleted recording remains in list")
		}
	}
	_, err = repo.Accessible(ctx, owner, cid, guestRecord.UUID)
	searchMust(t, err) // Other recordings are unaffected.
	for _, status := range []string{records.StatusReady, records.StatusPartialReady, records.StatusFailed, records.StatusCancelled} {
		terminal := create(status, author)
		_, err = repo.Delete(ctx, owner, cid, terminal.UUID, remove)
		searchMust(t, err)
	}

	t.Run("HTTP route auth validation and successful removal", func(t *testing.T) {
		router := gin.New()
		service := recordingsusecase.NewConferenceService(repo, nil, nil, recordingDeleteStorage(remove))
		transport.RegisterConferenceRecordingRoutes(router, recordingsapp.NewHandler(service), middleware.Authenticate(recordingDeleteTokens{}))
		target := create(records.StatusReady, author)
		base := "/api/v1/conferences/" + cid + "/recordings/"
		for _, tc := range []struct {
			user, id string
			status   int
		}{
			{"", target.UUID, 401}, {owner, "not-uuid", 422}, {viewer, target.UUID, 403},
			{owner, uuid.NewString(), 404}, {owner, target.UUID, 200}, {owner, target.UUID, 200},
		} {
			request := httptest.NewRequest(http.MethodDelete, base+tc.id, nil)
			if tc.user != "" {
				request.Header.Set("Authorization", "Bearer "+tc.user)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("DELETE expected %d got %d: %s", tc.status, response.Code, response.Body)
			}
		}
	})
}

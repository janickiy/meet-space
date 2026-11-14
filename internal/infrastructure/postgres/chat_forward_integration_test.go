package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/personal"
	"github.com/janickiy/meet-space/internal/domain/users"
	chatusecase "github.com/janickiy/meet-space/internal/usecase/chat"
)

// The database and attachment lifecycle are real; this private object store
// records actual independent bytes and validates their SHA-256 on Stat.
type forwardAuditObject struct {
	data []byte
	mime string
}
type forwardAuditStorage struct {
	chatusecase.Storage
	mu        sync.Mutex
	objects   map[string]forwardAuditObject
	failCopy  bool
	afterCopy func()
}

func (s *forwardAuditStorage) CheckAttachmentPrivacy(context.Context) error { return nil }
func (s *forwardAuditStorage) PutAttachment(_ context.Context, key string, r io.Reader, _ int64, mime, _ string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = forwardAuditObject{data, mime}
	return nil
}
func (s *forwardAuditStorage) CopyAttachment(_ context.Context, from, to string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.objects[from]
	if !ok {
		return errors.New("missing source")
	}
	s.objects[to] = forwardAuditObject{append([]byte(nil), o.data...), o.mime}
	if s.afterCopy != nil {
		s.afterCopy()
	}
	if s.failCopy {
		return errors.New("simulated timeout after object copy")
	}
	return nil
}
func (s *forwardAuditStorage) StatAttachment(_ context.Context, key string) (int64, string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.objects[key]
	if !ok {
		return 0, "", "", errors.New("missing object")
	}
	sum := sha256.Sum256(o.data)
	return int64(len(o.data)), o.mime, hex.EncodeToString(sum[:]), nil
}
func (s *forwardAuditStorage) CleanAttachmentObjects(_ context.Context, prefix, keep string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key := range s.objects {
		if strings.HasPrefix(key, prefix) && key != keep {
			delete(s.objects, key)
		}
	}
	return nil
}

func TestAuditForwardPermissionsIdempotencyAndAttachmentLifecycle(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	owner, author, receiver, outsider := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	for i, id := range []string{owner, author, receiver, outsider} {
		name := []string{"Owner", "Original author", "Receiver", "Outsider"}[i]
		if err := db.Create(&users.User{ID: id, Email: id + "@example.invalid", PasswordHash: "synthetic-not-a-login-hash", DisplayName: &name}).Error; err != nil {
			t.Fatal(err)
		}
	}
	personalRepo := NewPersonalRepository(db)
	source, _, err := personalRepo.CreateGroup(ctx, owner, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Source", MemberIDs: []string{author}})
	if err != nil {
		t.Fatal(err)
	}
	target, _, err := personalRepo.GetOrCreate(ctx, owner, receiver)
	if err != nil {
		t.Fatal(err)
	}
	store := &forwardAuditStorage{objects: map[string]forwardAuditObject{}}
	repo := NewDirectChatRepository(db)
	svc, err := chatusecase.NewService(ctx, repo, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	send := func(user, scope, text string, attachments ...string) chat.Message {
		t.Helper()
		req, fp, e := chat.NormalizeSend(chat.SendRequest{ClientRequestID: uuid.NewString(), Text: text, AttachmentIDs: attachments})
		if e != nil {
			t.Fatal(e)
		}
		m, _, e := repo.Send(ctx, user, scope, req, fp)
		if e != nil {
			t.Fatal(e)
		}
		return m
	}
	first := send(author, source.ID, "first")
	data := []byte("independently copied attachment\n")
	a, _, err := svc.InitAttachment(ctx, author, source.ID, chat.InitRequest{ClientRequestID: uuid.NewString(), Filename: "notes.txt", MimeType: "text/plain", Size: int64(len(data))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Upload(ctx, author, source.ID, a.ID, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.FinalizeAttachment(ctx, author, source.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	second := send(author, source.ID, "second", a.ID)
	request := chat.ForwardRequest{SourceType: "conversation", SourceID: source.ID, MessageIDs: []string{second.ID, first.ID}, ClientRequestID: uuid.NewString()}
	if _, _, err = svc.Forward(ctx, receiver, target.ID, request); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("source access not enforced", err)
	}
	if _, _, err = svc.Forward(ctx, author, target.ID, request); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("target access not enforced", err)
	}
	forwarded, created, err := svc.Forward(ctx, owner, target.ID, request)
	if err != nil || !created || len(forwarded) != 2 {
		t.Fatalf("forward: created=%v messages=%d err=%v", created, len(forwarded), err)
	}
	if forwarded[0].Text != "first" || forwarded[1].Text != "second" || forwarded[0].ForwardedFrom == nil || forwarded[0].ForwardedFrom.SenderName != "Original author" || forwarded[0].SenderID != owner {
		t.Fatal("wrong order/provenance", forwarded)
	}
	if _, err = repo.Edit(ctx, owner, target.ID, forwarded[0].ID, "changed attribution"); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("forwarded text can be edited under original author attribution", err)
	}
	if len(forwarded[1].Attachments) != 1 || forwarded[1].Attachments[0].ObjectKey == second.Attachments[0].ObjectKey || forwarded[1].Attachments[0].ID == a.ID {
		t.Fatal("attachment was not independently cloned")
	}
	dest := forwarded[1].Attachments[0]
	if !bytes.Equal(store.objects[dest.ObjectKey].data, data) {
		t.Fatal("cloned bytes differ")
	}
	if _, err = repo.Delete(ctx, owner, source.ID, first.ID); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("forwarding granted deletion of foreign message", err)
	}
	if _, err = repo.Delete(ctx, author, source.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Delete(ctx, author, source.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	// Simulate source retention removing the original object after deletion.
	if err = db.Exec("UPDATE conversation_attachments SET status='expired',message_id=NULL,expires_at=now()-interval '1 minute' WHERE id=?", a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = svc.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.objects[second.Attachments[0].ObjectKey]; ok {
		t.Fatal("deleted source attachment survived cleanup")
	}
	if !bytes.Equal(store.objects[dest.ObjectKey].data, data) {
		t.Fatal("source cleanup deleted forwarded attachment")
	}
	replayed, created, err := svc.Forward(ctx, owner, target.ID, request)
	if err != nil || created || replayed[0].ID != forwarded[0].ID {
		t.Fatal("retry after source deletion failed", err)
	}
	changed := request
	changed.MessageIDs = []string{first.ID}
	if _, _, err = svc.Forward(ctx, owner, target.ID, changed); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("changed idempotent payload accepted", err)
	}
	newRequest := request
	newRequest.ClientRequestID = uuid.NewString()
	if _, _, err = svc.Forward(ctx, owner, target.ID, newRequest); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("deleted source forwarded as new", err)
	}
	if _, _, err = svc.Forward(ctx, outsider, target.ID, request); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("foreign replay disclosed result", err)
	}
	if err = db.Exec("UPDATE conversation_members SET hidden_at=now() WHERE conversation_id=? AND user_id=?", target.ID, owner).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.Forward(ctx, owner, target.ID, request); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("revoked target replay accepted", err)
	}
	if err = db.Exec("UPDATE conversation_members SET hidden_at=NULL WHERE conversation_id=? AND user_id=?", target.ID, owner).Error; err != nil {
		t.Fatal(err)
	}
	self := chat.ForwardRequest{SourceType: "conversation", SourceID: target.ID, MessageIDs: []string{forwarded[0].ID}, ClientRequestID: uuid.NewString()}
	selfResult, _, err := svc.Forward(ctx, owner, target.ID, self)
	if err != nil || selfResult[0].ForwardedFrom.SenderName != "Original author" {
		t.Fatal("same-chat forward/provenance failed", err)
	}
	// A timeout after storage has copied bytes leaves a durable pending row;
	// the ordinary cleanup worker can remove its private prefix.
	store.failCopy = true
	failure := chat.ForwardRequest{SourceType: "conversation", SourceID: target.ID, MessageIDs: []string{forwarded[1].ID}, ClientRequestID: uuid.NewString()}
	if _, _, err = svc.Forward(ctx, owner, source.ID, failure); !errors.Is(err, apperrors.ErrUnavailable) {
		t.Fatal("copy timeout not propagated", err)
	}
	store.failCopy = false
	retried, wasCreated, err := svc.Forward(ctx, owner, source.ID, failure)
	if err != nil || !wasCreated || len(retried) != 1 {
		t.Fatal("copy timeout retry did not recover", err)
	}
	// A second failed batch is left for the normal TTL cleaner.
	failure.ClientRequestID = uuid.NewString()
	store.failCopy = true
	if _, _, err = svc.Forward(ctx, owner, source.ID, failure); !errors.Is(err, apperrors.ErrUnavailable) {
		t.Fatal("second copy timeout not propagated", err)
	}
	var pending chat.Attachment
	if err = repo.attachments(db).Where("client_request_id=?", chat.ForwardAttachmentRequestID(failure.ClientRequestID, dest.ID)).Take(&pending).Error; err != nil {
		t.Fatal(err)
	}
	if pending.Status != "pending" || pending.UploadLeaseUntil != nil {
		t.Fatal("failed copy has no durable cleanup row", pending.Status)
	}
	if err = db.Exec("UPDATE conversation_attachments SET expires_at=now()-interval '1 minute' WHERE id=?", pending.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = svc.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.objects[pending.ObjectKey]; ok {
		t.Fatal("failed-copy orphan survived cleanup")
	}
	store.failCopy = false
	// Access can be revoked during the external storage operation; no message
	// may be committed after the destination is hidden by its member.
	store.afterCopy = func() {
		if e := db.Exec("UPDATE conversation_members SET hidden_at=now() WHERE conversation_id=? AND user_id=?", target.ID, owner).Error; e != nil {
			t.Fatal(e)
		}
	}
	revoked := chat.ForwardRequest{SourceType: "conversation", SourceID: source.ID, MessageIDs: []string{retried[0].ID}, ClientRequestID: uuid.NewString()}
	if _, _, err = svc.Forward(ctx, owner, target.ID, revoked); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("target revoked during copying accepted", err)
	}
	store.afterCopy = nil
	if e := db.Exec("UPDATE conversation_members SET hidden_at=NULL WHERE conversation_id=? AND user_id=?", target.ID, owner).Error; e != nil {
		t.Fatal(e)
	}
	var revokedMessages int64
	if e := db.Table("conversation_messages").Where("client_request_id=?", chat.ForwardMessageRequestID(revoked.ClientRequestID, 0)).Count(&revokedMessages).Error; e != nil || revokedMessages != 0 {
		t.Fatal("revoked batch partially committed", e)
	}
	// Editing the original during object copying must reject the whole batch.
	fresh := send(author, source.ID, "before")
	editReq, fp, _ := chat.NormalizeForward(chat.ForwardRequest{SourceType: "conversation", SourceID: source.ID, MessageIDs: []string{fresh.ID}, ClientRequestID: uuid.NewString()})
	plan, err := repo.PrepareForward(ctx, owner, target.ID, editReq, fp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Edit(ctx, author, source.ID, fresh.ID, "after"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = repo.CommitForward(ctx, owner, target.ID, editReq, fp, plan.Messages, nil); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("changed source snapshot accepted", err)
	}
	// Opposing source/target lock order is stable and remains bounded.
	left := send(owner, source.ID, "left")
	right := send(owner, target.ID, "right")
	concurrent, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	errs := make(chan error, 2)
	for _, pair := range [][3]string{{source.ID, target.ID, left.ID}, {target.ID, source.ID, right.ID}} {
		go func(p [3]string) {
			_, _, e := svc.Forward(concurrent, owner, p[1], chat.ForwardRequest{SourceType: "conversation", SourceID: p[0], MessageIDs: []string{p[2]}, ClientRequestID: uuid.NewString()})
			errs <- e
		}(pair)
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal("opposing forward failed", err)
		}
	}
	// Simultaneous identical requests commit exactly once, with the same result.
	idempotent := chat.ForwardRequest{SourceType: "conversation", SourceID: source.ID, MessageIDs: []string{left.ID}, ClientRequestID: uuid.NewString()}
	type forwardResult struct {
		messages []chat.Message
		created  bool
		err      error
	}
	results := make(chan forwardResult, 2)
	for range 2 {
		go func() {
			m, c, e := svc.Forward(concurrent, owner, target.ID, idempotent)
			results <- forwardResult{m, c, e}
		}()
	}
	one, two := <-results, <-results
	if one.err != nil || two.err != nil || len(one.messages) != 1 || len(two.messages) != 1 || one.messages[0].ID != two.messages[0].ID || one.created == two.created {
		t.Fatalf("simultaneous retry duplicated or failed: first=%+v second=%+v", one, two)
	}
}

func TestAuditForwardFinishedConferenceAndHistoryBoundary(t *testing.T) {
	db := auditDatabase(t)
	ctx := context.Background()
	owner, member := uuid.NewString(), uuid.NewString()
	for _, id := range []string{owner, member} {
		if err := db.Create(&users.User{ID: id, Email: id + "@example.invalid", PasswordHash: "synthetic-not-a-login-hash"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	meeting := uuid.NewString()
	if err := db.Exec("INSERT INTO conferences(id,owner_id,title,invite_code,status,started_at) VALUES(?,?,'Forward audit',?,'active',now())", meeting, owner, strings.Repeat("b", 32)).Error; err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{owner, member} {
		if err := db.Exec("INSERT INTO conference_participants(id,conference_id,user_id,display_name,role,status,admission_state,joined_at) VALUES(?,?,?,'Audit user',?,'joined','admitted',now())", uuid.NewString(), meeting, id, []string{"owner", "participant"}[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	req, fp, _ := chat.NormalizeSend(chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "conference history"})
	m, _, err := NewChatRepository(db).Send(ctx, owner, meeting, req, fp)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec("UPDATE conferences SET status='finished',finished_at=now() WHERE id=?", meeting).Error; err != nil {
		t.Fatal(err)
	}
	target, _, err := NewPersonalRepository(db).GetOrCreate(ctx, owner, member)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := chatusecase.NewService(ctx, NewDirectChatRepository(db), &forwardAuditStorage{objects: map[string]forwardAuditObject{}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, _, err := svc.Forward(ctx, member, target.ID, chat.ForwardRequest{SourceType: "conference", SourceID: meeting, MessageIDs: []string{m.ID}, ClientRequestID: uuid.NewString()})
	if err != nil || len(result) != 1 {
		t.Fatal("read-only conference forwarding failed", err)
	}
	if err = db.Exec("UPDATE conversation_members SET history_cleared_through=? WHERE conversation_id=? AND user_id=?", result[0].Sequence, target.ID, owner).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.Forward(ctx, owner, target.ID, chat.ForwardRequest{SourceType: "conversation", SourceID: target.ID, MessageIDs: []string{result[0].ID}, ClientRequestID: uuid.NewString()}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("cleared source history readable", err)
	}
}

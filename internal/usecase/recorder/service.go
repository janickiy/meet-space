// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package recorder

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/records"
	"github.com/janickiy/meet-space/internal/infrastructure/ffmpeg"
	s3storage "github.com/janickiy/meet-space/internal/infrastructure/storage/s3"
)

// cardRepository reads metadata after conference-level authorization.
type cardRepository interface {
	FindDetailsByUUID(context.Context, string) (records.RecordDetails, error)
}

// workerRepository only reads persisted recording metadata before dispatch.
type workerRepository interface {
	FindByUUID(context.Context, string) (records.Record, error)
}

// recordingCommandHandler executes validated commands for the SFU recording pipeline.
type recordingCommandHandler interface {
	HandleCommand(context.Context, records.Command, records.Record) error
}

// conferenceReleaser releases a recording lock only for its current owner.
type conferenceReleaser interface {
	Release(context.Context, string, string) error
}

// Service builds file cards for already authorized conference recordings.
type Service struct {
	repository cardRepository
	s3         *s3storage.Client
}

// WorkerService validates queue commands and dispatches SFU conference recordings.
type WorkerService struct {
	repository workerRepository
	composite  recordingCommandHandler
}

// NewService creates a recording reader. repository supplies authorized metadata;
// s3 signs temporary URLs for stored recording artifacts.
func NewService(repository cardRepository, s3 *s3storage.Client) *Service {
	return &Service{repository: repository, s3: s3}
}

// NewWorkerService creates a command dispatcher. repository supplies stored
// recording metadata; composite owns capture, finalization, and publication.
func NewWorkerService(repository workerRepository, composite recordingCommandHandler) *WorkerService {
	return &WorkerService{repository: repository, composite: composite}
}

// HandleCommand validates command and its persisted recording before dispatch.
// ctx bounds database access and command handling. command contains the record
// UUID and start/stop operation received from the private queue. Unsupported
// recordings return an error without changing their metadata or stored files.
func (s *WorkerService) HandleCommand(ctx context.Context, command records.Command) error {
	id, err := uuid.Parse(command.RecordID)
	if err != nil {
		return fmt.Errorf("recordId must be valid UUID: %w", err)
	}
	command.RecordID = id.String()
	if command.Type != "record.start" && command.Type != "record.stop" {
		return fmt.Errorf("unknown command type %q", command.Type)
	}
	record, err := s.repository.FindByUUID(ctx, command.RecordID)
	if err != nil {
		return err
	}
	if !records.IsSFURecording(record) {
		return records.ErrUnsupportedRecording
	}
	if s.composite == nil {
		return fmt.Errorf("SFU recording processor is not configured")
	}
	return s.composite.HandleCommand(ctx, command, record)
}

// recordCard создает карточку записи с соответствующими артефактами и временно считывает URLs.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - details: Метаданные записи и связанные артефакты форматируются.
//
// @returns:
//   - result1: Полученная карта записи.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) recordCard(ctx context.Context, details records.RecordDetails) (records.RecordCard, error) {
	card := records.RecordCard{
		Record:   details.Record,
		Files:    make([]records.RecordFileView, 0, len(details.Files)),
		Segments: make([]records.RecordSegmentView, 0, len(details.Segments)),
		Events:   details.Events,
	}
	for _, file := range details.Files {
		view := records.RecordFileView{RecordFile: file}
		url, err := s.presignedURL(ctx, file.ObjectKey)
		if err != nil {
			return records.RecordCard{}, err
		}
		view.URL = url
		card.Files = append(card.Files, view)
	}

	for _, segment := range details.Segments {
		view := records.RecordSegmentView{RecordSegment: segment}
		if segment.ObjectKey != nil {
			if details.Record.StorageBucket != nil {
				view.Bucket = *details.Record.StorageBucket
			} else if s.s3 != nil {
				view.Bucket = s.s3.Bucket()
			}
			url, err := s.presignedURL(ctx, *segment.ObjectKey)
			if err != nil {
				return records.RecordCard{}, err
			}
			view.URL = url
		}
		card.Segments = append(card.Segments, view)
	}

	return card, nil
}

// presignedURL создает временный объект чтения URL для частного артефакта записи.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - objectKey: Сгенерированный сервером закрытый ключ для хранения объектов.
//
// @returns:
//   - result1: Временный подписанный объект чтения URL для артефакта записи.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) presignedURL(ctx context.Context, objectKey string) (string, error) {
	if s.s3 == nil || objectKey == "" {
		return "", nil
	}

	return s.s3.PresignedGetURL(ctx, objectKey, 24*time.Hour)
}

// segmentMetadata преобразует локальный сегмент медиа в сохраняемые метаданные записи.
//
// @Parameters:
//   - segments: Упорядоченные сегменты медиаданных.
//
// @returns:
//   - result1: Собранные значения сегментов записи.
func segmentMetadata(segments []ffmpeg.Segment) []records.RecordSegment {
	result := make([]records.RecordSegment, 0, len(segments))
	for _, segment := range segments {
		fileName := segment.FileName
		sizeBytes := segment.SizeBytes
		checksum := segment.ChecksumSHA256
		result = append(result, records.RecordSegment{
			SeqNo:          segment.SeqNo,
			Status:         "closed",
			FileName:       &fileName,
			SizeBytes:      &sizeBytes,
			ChecksumSHA256: &checksum,
		})
	}

	return result
}

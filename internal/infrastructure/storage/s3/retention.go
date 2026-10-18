package s3

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/records"
	"github.com/minio/minio-go/v7"
)

// recordingPrefixes получает узкие пространства имен из канонического UUIDs, а не из
// произвольного сохраненного ключа объекта.  Это исключает вложения в чатах и в других
// комнатах.
//
// @Parameters:
//   - record: Запись и сохраненные метаданные ее жизненного цикла.
//
// @returns:
//   - result1: Собранные строковые значения.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func recordingPrefixes(record records.Record) ([]string, error) {
	rid, err := uuid.Parse(record.UUID)
	if err != nil || rid.String() != record.UUID {
		return nil, fmt.Errorf("invalid recording UUID")
	}
	prefixes := []string{"records/" + record.UUID + "/"}
	if records.IsComposite(record) {
		cid, err := uuid.Parse(record.ConferenceID)
		if err != nil || cid.String() != record.ConferenceID {
			return nil, fmt.Errorf("invalid recording conference UUID")
		}
		prefixes = append(prefixes, "recordings/"+record.ConferenceID+"/"+record.UUID+"/")
	}
	return prefixes, nil
}

// RemoveRecording удаляет все артефакты, включая исторические версии объектов.  При повторной
// попытке отсутствующие объекты не будут затронуты; ошибки при извлечении и удалении
// распространяются.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - record: Запись и сохраненные метаданные ее жизненного цикла.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Client) RemoveRecording(ctx context.Context, record records.Record) error {
	if record.StorageBucket != nil && *record.StorageBucket != c.bucket {
		return fmt.Errorf("recording storage bucket mismatch")
	}
	prefixes, err := recordingPrefixes(record)
	if err != nil {
		return err
	}
	listing, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, prefix := range prefixes {
		for object := range c.minio.ListObjectsIter(listing, c.bucket, minio.ListObjectsOptions{
			Prefix: prefix, Recursive: true, WithVersions: true,
		}) {
			if object.Err != nil {
				return fmt.Errorf("list recording artifacts: %w", object.Err)
			}
			if err := c.minio.RemoveObject(ctx, c.bucket, object.Key, minio.RemoveObjectOptions{VersionID: object.VersionID}); err != nil {
				return fmt.Errorf("remove recording artifact: %w", err)
			}
		}
	}
	return ctx.Err()
}

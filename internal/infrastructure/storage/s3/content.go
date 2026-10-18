package s3

import (
	"context"
	"fmt"
	"github.com/minio/minio-go/v7"
	"io"
	"strings"
)

// limitedRecording проверяет лимит на чтение и закрывает поток для приватных объектов.
type limitedRecording struct {
	reader io.Reader
	object io.Closer
}

// Read — прочитанные копии имеют размер, не превышающий размер буфера назначения, и извлекаются из
// потока, при этом сохраняются ограничения на количество прочитанных данных и возможность
// отмены операции.
//
// @Parameters:
//   - p: Целевой байтовый буфер заполняется данными из потока.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *limitedRecording) Read(p []byte) (int, error) { return r.reader.Read(p) }

// Close закрывает ресурсы, принадлежащие компоненту, и завершает их жизненный цикл.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *limitedRecording) Close() error { return r.object.Close() }

// OpenRecording открывает приватный аудиопоток или медиапоток для записи, используя сохраненный
// идентификатор артефакта.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//   - maximum: Максимально допустимый размер или количество символов.
//
// @returns:
//   - result1: Результирующий считывающий модуль.
//   - result2: Возвращает количество, размер или числовое значение.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Client) OpenRecording(ctx context.Context, key string, maximum int64) (io.ReadCloser, int64, error) {
	parts := strings.Split(key, "/")
	if maximum <= 0 || len(parts) != 6 || parts[0] != "recordings" || !attachmentID(parts[1]) || !attachmentID(parts[2]) || parts[3] != "artifacts" || !attachmentID(parts[4]) || parts[5] != "final.mp4" {
		return nil, 0, fmt.Errorf("invalid private recording key")
	}
	object, err := c.minio.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("private recording unavailable")
	}
	info, err := object.Stat()
	if err != nil {
		object.Close()
		return nil, 0, fmt.Errorf("private recording unavailable")
	}
	if info.Size <= 0 || info.Size > maximum {
		object.Close()
		return nil, 0, fmt.Errorf("recording input exceeds budget")
	}
	return &limitedRecording{reader: io.LimitReader(object, maximum+1), object: object}, info.Size, nil
}

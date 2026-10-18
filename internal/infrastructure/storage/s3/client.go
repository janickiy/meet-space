package s3

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client — клиент владеет настроенными частными клиентами объектного хранилища и параметрами
// подключения.
type Client struct {
	minio        *minio.Client
	presignMinio *minio.Client
	bucket       string
	endpoint     string
	accessKey    string
	secretKey    string
	useSSL       bool
}

// UploadedObject содержит сохранённый ключ, размер и метаданные загруженного артефакта записи.
type UploadedObject struct {
	Bucket    string
	ObjectKey string
	SizeBytes int64
}

// CompletedRecord группирует конечные объекты, найденные в бакет-хранилище, для одной
// завершённой записи.
type CompletedRecord struct {
	RecordID         string    `json:"recordId"`
	Status           string    `json:"status"`
	Bucket           string    `json:"bucket"`
	PreviewObjectKey string    `json:"previewObjectKey,omitempty"`
	PreviewURL       string    `json:"previewUrl,omitempty"`
	FinalObjectKey   string    `json:"finalObjectKey,omitempty"`
	FinalURL         string    `json:"finalUrl,omitempty"`
	SizeBytes        int64     `json:"sizeBytes,omitempty"`
	LastModified     time.Time `json:"lastModified,omitempty"`
}

// NewClient создаёт клиент с зависимостями и ограничениями, необходимыми для частного хранения
// записей и вложений.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - endpoint: Конечная точка сервиса, настраиваемая оператором.
//   - accessKey: Идентификатор ключа доступа к частному объектному хранилищу.
//   - secretKey: Секрет подписи для частного объектного хранилища; никогда не записывайте его и
//     не раскрывайте.
//   - bucket: Бакет-хранилище для частного объектного хранилища.
//   - useSSL: Должен ли клиент объектного хранилища использовать TLS.
//
// @returns:
//   - result1: Созданный или загруженный клиент.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func NewClient(ctx context.Context, endpoint string, accessKey string, secretKey string, bucket string, useSSL bool) (*Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	client, err := minio.New(endpoint, &minio.Options{
		Creds:      credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:     useSSL,
		Transport:  boundedTransport(),
		MaxRetries: 3,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("check minio bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create minio bucket: %w", err)
		}
	}

	return &Client{
		minio:     client,
		bucket:    bucket,
		endpoint:  endpoint,
		accessKey: accessKey,
		secretKey: secretKey,
		useSSL:    useSSL,
	}, nil
}

// boundedTransport создаёт транспортный протокол HTTP с ограничением времени ожидания
// подключения и запросов.
//
// @returns:
//   - result1: Созданный или загруженный транспортный протокол.
func boundedTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	t.ResponseHeaderTimeout = 10 * time.Second
	t.TLSHandshakeTimeout = 5 * time.Second
	t.MaxIdleConnsPerHost = 8
	return t
}

// Check проверяет настроенный компонент или зависимость без создания пользовательских задач.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Client) Check(ctx context.Context) error {
	exists, err := c.minio.BucketExists(ctx, c.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("storage bucket unavailable")
	}
	return nil
}

// SetPublicEndpoint обновляет общедоступную конечную точку для частного хранения записей и
// вложений.
//
// @Parameters:
//   - endpoint: Конечная точка сервиса, настраиваемая оператором.
func (c *Client) SetPublicEndpoint(endpoint string) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		c.presignMinio = nil
		return
	}
	secure := c.useSSL
	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Host == "" {
			c.presignMinio = nil
			return
		}
		endpoint = parsed.Host
		secure = parsed.Scheme == "https"
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(c.accessKey, c.secretKey, ""),
		Secure:    secure,
		Region:    "us-east-1",
		Transport: c.publicEndpointTransport(endpoint),
	})
	if err != nil {
		c.presignMinio = nil
		return
	}
	c.presignMinio = client
}

// publicEndpointTransport перезаписывает только настроенный источник объектного хранилища для
// подписанного общедоступного URLs.
//
// @Parameters:
//   - publicEndpoint: Настроенный оператором источник объектного хранилища.
//
// @returns:
//   - result1: Результирующий обмен данными в обе стороны.
func (c *Client) publicEndpointTransport(publicEndpoint string) http.RoundTripper {
	transport := boundedTransport()
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	internalEndpoint := c.endpoint

	transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		if address == publicEndpoint && internalEndpoint != "" {
			address = internalEndpoint
		}

		return dialer.DialContext(ctx, network, address)
	}

	return transport
}

// Bucket возвращает настроенный адрес частного хранилища, используемый этим клиентом хранилища.
//
// @returns:
//   - result1: Настроенный адрес частного хранилища объектов.
func (c *Client) Bucket() string {
	return c.bucket
}

// UploadFile загружает локальный артефакт записи и возвращает метаданные сохраненного объекта.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - objectKey: Сгенерированный сервером закрытый ключ для хранения объектов.
//   - path: Проверенный путь к локальной файловой системе или объекту.
//   - contentType: Объявленный или обнаруженный тип контента MIME.
//
// @returns:
//   - result1: Результирующий загруженный объект.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Client) UploadFile(ctx context.Context, objectKey string, path string, contentType string) (UploadedObject, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	local, err := os.Stat(path)
	if err != nil {
		return UploadedObject{}, err
	}
	info, err := c.minio.FPutObject(ctx, c.bucket, objectKey, path, minio.PutObjectOptions{
		ContentType: contentType,
		UserMetadata: map[string]string{
			"file-name": filepath.Base(path),
		},
	})
	if err != nil {
		return UploadedObject{}, fmt.Errorf("upload %s to minio: %w", objectKey, err)
	}
	verified, err := c.minio.StatObject(ctx, c.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil || verified.Size != local.Size() {
		return UploadedObject{}, fmt.Errorf("uploaded object verification failed")
	}

	return UploadedObject{Bucket: c.bucket, ObjectKey: objectKey, SizeBytes: info.Size}, nil
}

// RemovePrefix удаляет объекты только в пределах валидированного префикса записи,
// принадлежащего серверу.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - prefix: Объект или префикс ключа, управляемый сервером.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Client) RemovePrefix(ctx context.Context, prefix string) error {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Синхронные итераторы останавливаются вместе с потребителем.  Канал SDK APIs может
	// сохранять горутины, отвечающие за вывод списка или результатов, до тех пор, пока при
	// удалении не возникнет первая ошибка.
	var listingErr error
	objects := func(yield func(minio.ObjectInfo) bool) {
		for object := range c.minio.ListObjectsIter(ctx, c.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
			// Итератор массового удаления не распространяет ошибку ObjectInfo.Err.
			if object.Err != nil {
				listingErr = object.Err
				return
			}
			if !yield(object) {
				return
			}
		}
	}
	results, err := c.minio.RemoveObjectsWithIter(ctx, c.bucket, objects, minio.RemoveObjectsOptions{})
	if err != nil {
		return fmt.Errorf("remove prefix from minio: %w", err)
	}
	for removeErr := range results {
		if removeErr.Err != nil {
			return fmt.Errorf("remove %s from minio: %w", removeErr.ObjectName, removeErr.Err)
		}
	}
	if listingErr != nil {
		return fmt.Errorf("list prefix from minio: %w", listingErr)
	}
	return ctx.Err()
}

// ListCompletedRecords выводит список завершенных артефактов записи, хранящихся под настроенным
// приватным префиксом.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - limit: Максимальное количество результатов или операций в этом пакете.
//
// @returns:
//   - result1: Собранные значения завершенных записей.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Client) ListCompletedRecords(ctx context.Context, limit int) ([]CompletedRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	byRecordID := make(map[string]*CompletedRecord)
	objects := c.minio.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{
		Prefix:    "records/",
		Recursive: true,
	})
	for object := range objects {
		if object.Err != nil {
			return nil, fmt.Errorf("list completed records from minio: %w", object.Err)
		}
		recordID, fileName, ok := completedRecordObject(object.Key)
		if !ok {
			continue
		}
		item := byRecordID[recordID]
		if item == nil {
			item = &CompletedRecord{
				RecordID: recordID,
				Status:   "ready",
				Bucket:   c.bucket,
			}
			byRecordID[recordID] = item
		}
		if object.LastModified.After(item.LastModified) {
			item.LastModified = object.LastModified
		}
		if fileName == "final.mp4" {
			item.FinalObjectKey = object.Key
			item.SizeBytes = object.Size
		}
		if fileName == "preview.jpg" {
			item.PreviewObjectKey = object.Key
		}
	}

	result := make([]CompletedRecord, 0, len(byRecordID))
	for _, item := range byRecordID {
		if item.FinalObjectKey == "" || item.PreviewObjectKey == "" {
			continue
		}
		if item.FinalObjectKey != "" {
			url, err := c.PresignedGetURL(ctx, item.FinalObjectKey, 24*time.Hour)
			if err != nil {
				return nil, err
			}
			item.FinalURL = url
		}
		if item.PreviewObjectKey != "" {
			url, err := c.PresignedGetURL(ctx, item.PreviewObjectKey, 24*time.Hour)
			if err != nil {
				return nil, err
			}
			item.PreviewURL = url
		}
		result = append(result, *item)
	}
	sort.Slice(result,

		func(i int, j int) bool {
			return result[i].LastModified.After(result[j].LastModified)
		})
	if len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

// completedRecordObject распознает ключи объектов, принадлежащие завершенному артефакту записи.
//
// @Parameters:
//   - key: Хранилище, кэш или криптографический ключ, используемый в ходе операции.
//
// @returns:
//   - result1: Идентификатор записи, декодированный из ключа объекта.
//   - result2: Распознанное имя файла артефакта final.mp4 или preview.jpg.
//   - result3: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func completedRecordObject(key string) (string, string, bool) {
	parts := strings.Split(strings.Trim(key, "/"), "/")
	if len(parts) != 3 || parts[0] != "records" {
		return "", "", false
	}
	if parts[2] != "final.mp4" && parts[2] != "preview.jpg" {
		return "", "", false
	}

	return parts[1], parts[2], true
}

// PresignedGetURL создает временный подписанный URL для чтения приватного объекта.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - objectKey: Сгенерированный сервером закрытый ключ для хранения объектов.
//   - expiry: Срок действия или интервал действительности, определяемый типом.
//
// @returns:
//   - result1: Временный подписанный URL для приватного объекта.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (c *Client) PresignedGetURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	if strings.TrimSpace(objectKey) == "" {
		return "", nil
	}
	client := c.minio
	if c.presignMinio != nil {
		client = c.presignMinio
	}
	objectURL, err := client.PresignedGetObject(ctx, c.bucket, objectKey, expiry, url.Values{})
	if err != nil {
		return "", fmt.Errorf("presign %s: %w", objectKey, err)
	}

	return objectURL.String(), nil
}

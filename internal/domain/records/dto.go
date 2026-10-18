package records

// Command содержит данные команд, используемые для метаданных записи.
type Command struct {
	Type               string `json:"type"`
	RecordID           string `json:"recordId"`
	Reason             string `json:"reason,omitempty"`
	SegmentDurationSec int    `json:"segmentDurationSec,omitempty"`
}

// WebRTCOfferRequest содержит предложение клиента SDP, используемое для организации приема
// записи.
type WebRTCOfferRequest struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// WebRTCAnswerResponse содержит согласованный ответ получателя записи SDP.
type WebRTCAnswerResponse struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// RecordDetails объединяет запись с сопутствующими файлами, сегментами и событиями для чтения
// приложениями.
type RecordDetails struct {
	Record   Record
	Files    []RecordFile
	Segments []RecordSegment
	Events   []RecordEvent
}

// RecordCard содержит авторизованное представление записи с разрешенными ссылками и
// сопутствующими артефактами.
type RecordCard struct {
	Record
	Files    []RecordFileView    `json:"files"`
	Segments []RecordSegmentView `json:"segments"`
	Events   []RecordEvent       `json:"events,omitempty"`
}

// RecordFileView добавляет временную авторизованную ссылку для чтения к метаданным сохраненного
// файла.
type RecordFileView struct {
	RecordFile
	URL string `json:"url,omitempty"`
}

// RecordSegmentView добавляет приватный сегмент и временную ссылку для чтения к метаданным
// сегмента.
type RecordSegmentView struct {
	RecordSegment
	Bucket string `json:"bucket,omitempty"`
	URL    string `json:"url,omitempty"`
}

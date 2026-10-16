package records

// Command задаёт согласованное представление данных «Command» для управлении задачами записи и её артефактами.
// @params
//   - Type: значение Type типа string, используемое согласно назначению этой операции.
//   - RecordID: внешний UUID задачи записи.
//   - Reason: причина завершения, отказа или изменения состояния.
//   - SegmentDurationSec: плановая длительность сегмента записи в секундах.
type Command struct {
	Type               string `json:"type"`
	RecordID           string `json:"recordId"`
	Reason             string `json:"reason,omitempty"`
	SegmentDurationSec int    `json:"segmentDurationSec,omitempty"`
}

// WebRTCOfferRequest передаёт SDP-предложение клиента при установке входа записи.
//   - Type: значение Type типа string, используемое согласно назначению этой операции.
//   - SDP: значение SDP типа string, используемое согласно назначению этой операции.
type WebRTCOfferRequest struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// WebRTCAnswerResponse возвращает SDP-ответ приёмника записи.
//   - Type: значение Type типа string, используемое согласно назначению этой операции.
//   - SDP: значение SDP типа string, используемое согласно назначению этой операции.
type WebRTCAnswerResponse struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// RecordDetails объединяет запись и связанные файлы, сегменты и события для прикладного чтения.
// @params
//   - Record: задача записи с её сохранённым состоянием.
//   - Files: набор файлов или артефактов для обработки.
//   - Segments: доступные сегменты записи для итоговой сборки.
//   - Events: получатель или издатель событий прикладного сценария.
type RecordDetails struct {
	Record   Record
	Files    []RecordFile
	Segments []RecordSegment
	Events   []RecordEvent
}

// RecordCard возвращает карточку записи с разрешёнными ссылками и связанными артефактами.
// @params
//   - Record: встроенный тип, добавляющий свой контракт или данные.
//   - Files: набор файлов или артефактов для обработки.
//   - Segments: доступные сегменты записи для итоговой сборки.
//   - Events: получатель или издатель событий прикладного сценария.
type RecordCard struct {
	Record
	Files    []RecordFileView    `json:"files"`
	Segments []RecordSegmentView `json:"segments"`
	Events   []RecordEvent       `json:"events,omitempty"`
}

// RecordFileView добавляет к метаданным файла временную ссылку разрешённого чтения.
// @params
//   - RecordFile: встроенный тип, добавляющий свой контракт или данные.
//   - URL: значение URL типа string, используемое согласно назначению этой операции.
type RecordFileView struct {
	RecordFile
	URL string `json:"url,omitempty"`
}

// RecordSegmentView добавляет к метаданным сегмента бакет и временную ссылку чтения.
// @params
//   - RecordSegment: встроенный тип, добавляющий свой контракт или данные.
//   - Bucket: имя бакета объектного хранилища.
//   - URL: значение URL типа string, используемое согласно назначению этой операции.
type RecordSegmentView struct {
	RecordSegment
	Bucket string `json:"bucket,omitempty"`
	URL    string `json:"url,omitempty"`
}

package admin

import "time"

type Conference struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	OwnerName        string     `json:"ownerName"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"createdAt"`
	StartedAt        *time.Time `json:"startedAt"`
	EndedAt          *time.Time `json:"endedAt"`
	ParticipantCount int64      `json:"participantCount"`
	RecordingStatus  *string    `json:"recordingStatus"`
}

type Recording struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	ConferenceID    *string   `json:"conferenceId"`
	ConferenceTitle *string   `json:"conferenceTitle"`
	Status          string    `json:"status"`
	DurationSeconds *int64    `json:"durationSeconds"`
	SizeBytes       *int64    `json:"sizeBytes"`
	CreatedAt       time.Time `json:"createdAt"`
	CanDelete       bool      `json:"canDelete"`
}

type FileCategory struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
	Bytes *int64 `json:"bytes"`
}
type Files struct {
	Count         int64          `json:"count"`
	Bytes         *int64         `json:"bytes"`
	CapacityBytes *int64         `json:"capacityBytes"`
	Categories    []FileCategory `json:"categories"`
	Scope         string         `json:"scope"`
}
type Chats struct {
	DirectChats     int64 `json:"directChats"`
	GroupChats      int64 `json:"groupChats"`
	ConferenceChats int64 `json:"conferenceChats"`
	Messages        int64 `json:"messages"`
	Attachments     int64 `json:"attachments"`
}
type ServiceHealth struct {
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	LatencyMS *int64     `json:"latencyMs"`
	Version   *string    `json:"version"`
	CheckedAt *time.Time `json:"checkedAt"`
}
type SystemResources struct {
	CPUPercent       *float64 `json:"cpuPercent"`
	MemoryUsedBytes  *int64   `json:"memoryUsedBytes"`
	MemoryTotalBytes *int64   `json:"memoryTotalBytes"`
	DiskUsedBytes    *int64   `json:"diskUsedBytes"`
	DiskTotalBytes   *int64   `json:"diskTotalBytes"`
}
type System struct {
	Services  []ServiceHealth `json:"services"`
	Resources SystemResources `json:"resources"`
}
type DashboardUsers struct {
	Total   int64 `json:"total"`
	Active  int64 `json:"active"`
	Blocked int64 `json:"blocked"`
	New     int64 `json:"new"`
}
type DashboardConferences struct {
	Total        int64 `json:"total"`
	Active       int64 `json:"active"`
	Completed    int64 `json:"completed"`
	Participants int64 `json:"participants"`
}
type DashboardRecordings struct {
	Total  int64  `json:"total"`
	Active int64  `json:"active"`
	Bytes  *int64 `json:"bytes"`
}
type ActivityPoint struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}
type Dashboard struct {
	Users       DashboardUsers       `json:"users"`
	Conferences DashboardConferences `json:"conferences"`
	Recordings  DashboardRecordings  `json:"recordings"`
	Files       Files                `json:"files"`
	Services    []ServiceHealth      `json:"services"`
	Activity    []ActivityPoint      `json:"activity"`
	CheckedAt   time.Time            `json:"checkedAt"`
}

// Settings are an explicit allowlist of safe, effective configuration values.
// Secret settings only expose whether a value is configured.
type Setting struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Value       any    `json:"value"`
	Editable    bool   `json:"editable"`
	Type        string `json:"type"`
	Group       string `json:"group"`
	Secret      bool   `json:"secret"`
	Configured  bool   `json:"configured"`
}
type Settings struct {
	Items []Setting `json:"items"`
}

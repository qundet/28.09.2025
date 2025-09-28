package main

import "time"

// FileStatusState - перечисление статусов для отдельного файла
type FileStatusState string

const (
	Pending    FileStatusState = "pending"
	InProgress FileStatusState = "in_progress"
	Done       FileStatusState = "done"
	Failed     FileStatusState = "failed"
)

// FileStatus - состояние конкретного URL в задаче
type FileStatus struct {
	URL      string          `json:"url"`
	FileName string          `json:"file_name"`
	State    FileStatusState `json:"state"`
	Error    string          `json:"error,omitempty"`
	Size     int64           `json:"size_bytes,omitempty"`
}

// Task - задача пользователя
type Task struct {
	ID        string       `json:"id"`
	Name      string       `json:"name,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	Files     []FileStatus `json:"files"`
}

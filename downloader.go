package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Manager отвечает за очередь задач и воркеры
type Manager struct {
	store      *Store
	queue      chan string // task IDs
	workers    int
	dataDir    string
	wg         sync.WaitGroup
	httpClient *http.Client
}

func NewManager(store *Store, workers int, dataDir string) *Manager {
	return &Manager{
		store:      store,
		queue:      make(chan string, 100),
		workers:    workers,
		dataDir:    dataDir,
		httpClient: &http.Client{Timeout: 0},
	}
}

func (m *Manager) Start(ctx context.Context) {
	for i := 0; i < m.workers; i++ {
		m.wg.Add(1)
		go m.worker(ctx)
	}
}

func (m *Manager) Stop() {
	// закрываем очередь: это позволит воркерам уйти, когда обработают все задания
	close(m.queue)
	m.wg.Wait()
}

func (m *Manager) Enqueue(taskID string) {
	// non-blocking try
	select {
	case m.queue <- taskID:
	default:
		// если очередь переполнена, положим с блокировкой — проще, чем отбрасывать
		m.queue <- taskID
	}
}

func (m *Manager) worker(ctx context.Context) {
	defer m.wg.Done()
	for taskID := range m.queue {
		// получаем задачу
		t, ok := m.store.GetTask(taskID)
		if !ok {
			continue
		}
		// проходим по файлам задачи
		for i := range t.Files {
			f := &t.Files[i]
			if f.State == Done {
				continue
			}
			// отметить in_progress и сохранить
			f.State = InProgress
			f.Error = ""
			if err := m.store.UpdateTask(t); err != nil {
				// логируем и ставим failed
				f.State = Failed
				f.Error = err.Error()
				_ = m.store.UpdateTask(t)
				continue
			}
			if err := m.downloadFile(ctx, t.ID, f); err != nil {
				f.State = Failed
				f.Error = err.Error()
				_ = m.store.UpdateTask(t)
				continue
			}
			f.State = Done
			_ = m.store.UpdateTask(t)
		}
	}
}

func (m *Manager) downloadFile(ctx context.Context, taskID string, f *FileStatus) error {
	// формируем директорию
	taskDir := filepath.Join(m.dataDir, taskID)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return err
	}

	// filename: берем базовое имя из URL
	filename := filepath.Base(f.URL)
	if filename == "" || filename == "." || filename == "/" {
		// fallback
		filename = fmt.Sprintf("file_%d", time.Now().Unix())
	}
	f.FileName = filename

	tmpPath := filepath.Join(taskDir, filename+".part")
	finalPath := filepath.Join(taskDir, filename)

	// HTTP GET
	req, err := http.NewRequestWithContext(ctx, "GET", f.URL, nil)
	if err != nil {
		return err
	}
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// create temp file
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	// копируем
	n, err := io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		return err
	}
	f.Size = n
	// переименовываем
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return err
	}
	return nil
}

// ResumePending помещает в очередь все задачи, у которых есть незавершённые файлы
func (m *Manager) ResumePending() {
	tasks := m.store.ListTasks()
	for _, t := range tasks {
		need := false
		for _, f := range t.Files {
			if f.State != Done {
				need = true
				break
			}
		}
		if need {
			m.Enqueue(t.ID)
		}
	}
}

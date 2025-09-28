package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Store — хранит все задачи в памяти и сериализует на диск
type Store struct {
	Path  string
	mu    sync.Mutex
	Tasks map[string]Task
}

func NewStore(path string) (*Store, error) {
	s := &Store{Path: path, Tasks: make(map[string]Task)}
	if err := s.load(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return err
	}
	var tasks map[string]Task
	if err := json.Unmarshal(b, &tasks); err != nil {
		return err
	}
	s.Tasks = tasks
	return nil
}

func (s *Store) save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tmp := s.Path + ".tmp"
	b, err := json.MarshalIndent(s.Tasks, "", " ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

func (s *Store) AddTask(t Task) error {
	s.mu.Lock()
	s.Tasks[t.ID] = t
	s.mu.Unlock()
	return s.save()
}

func (s *Store) UpdateTask(t Task) error {
	s.mu.Lock()
	s.Tasks[t.ID] = t
	s.mu.Unlock()
	return s.save()
}

func (s *Store) GetTask(id string) (Task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.Tasks[id]
	return t, ok
}

func (s *Store) ListTasks() []Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Task, 0, len(s.Tasks))
	for _, t := range s.Tasks {
		out = append(out, t)
	}
	return out
}

func ensureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0755)
}

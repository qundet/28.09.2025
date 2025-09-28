package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func genID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func main() {
	dataDir := "data"
	storePath := "tasks.json"
	// ensure data dir
	_ = os.MkdirAll(dataDir, 0755)

	store, err := NewStore(storePath)
	if err != nil {
		log.Fatalf("load store: %v", err)
	}

	mgr := NewManager(store, 4, dataDir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)

	// resume pending
	mgr.ResumePending()

	mux := http.NewServeMux()

	mux.HandleFunc("/tasks", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var req struct {
				Name string   `json:"name"`
				URLs []string `json:"urls"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if len(req.URLs) == 0 {
				http.Error(w, "empty urls", http.StatusBadRequest)
				return
			}
			id := genID()
			files := make([]FileStatus, 0, len(req.URLs))
			for _, u := range req.URLs {
				files = append(files, FileStatus{URL: u, State: Pending})
			}
			task := Task{ID: id, Name: req.Name, CreatedAt: time.Now(), Files: files}
			if err := store.AddTask(task); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// enqueue for processing
			mgr.Enqueue(id)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(task)
			return
		case http.MethodGet:
			// list
			list := store.ListTasks()
			_ = json.NewEncoder(w).Encode(list)
			return
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/tasks/", func(w http.ResponseWriter, r *http.Request) {
		id := filepath.Base(r.URL.Path)
		t, ok := store.GetTask(id)
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(t)
	})

	srv := &http.Server{Addr: ":8080", Handler: mux}

	// сигнал-обработчик
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Println("listening :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	<-stop
	log.Println("shutting down...")
	// graceful HTTP shutdown
	ctxShut, cancelShut := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShut()
	if err := srv.Shutdown(ctxShut); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	// закрываем менеджер: даём ему завершиться
	mgr.Stop()
	// финальное сохранение
	if err := store.save(); err != nil {
		log.Printf("save store: %v", err)
	}
	log.Println("bye")
}

package logging

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
)

var logFile *os.File

func InitGoLogger() error {
	logDir := filepath.Join("logs", "go")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	file, err := os.OpenFile(filepath.Join(logDir, "gopherai.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	logFile = file

	writer := io.MultiWriter(os.Stdout, file)
	log.SetOutput(writer)
	slog.SetDefault(slog.New(slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))
	return nil
}

package main

import (
	"io"
	"log/slog"
	"os"
)

type Logger struct {
	*slog.Logger
	file *os.File
}

func newLogger(cfg LogConfig) (*Logger, error) {
	if !cfg.Enabled {
		return &Logger{
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		}, nil
	}

	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "info", "":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	var w io.Writer
	var f *os.File
	if cfg.File != "" {
		var err error
		f, err = os.OpenFile(cfg.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return nil, err
		}
		w = f
	} else {
		w = os.Stderr
	}

	return &Logger{
		Logger: slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})),
		file:   f,
	}, nil
}

func (l *Logger) Close() {
	if l.file != nil {
		l.file.Close()
	}
}

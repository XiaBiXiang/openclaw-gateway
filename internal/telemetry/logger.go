package telemetry

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"
)

type Logger struct {
	base  *log.Logger
	level string
}

func New(level string) *Logger {
	normalized := strings.ToLower(strings.TrimSpace(level))
	if normalized == "" {
		normalized = "info"
	}

	return &Logger{
		base:  log.New(os.Stdout, "", 0),
		level: normalized,
	}
}

func (l *Logger) Info(message string, attrs map[string]any) {
	l.write("info", message, attrs)
}

func (l *Logger) Error(message string, attrs map[string]any) {
	l.write("error", message, attrs)
}

func (l *Logger) write(level, message string, attrs map[string]any) {
	payload := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339),
		"level": level,
		"msg":   message,
	}

	for key, value := range attrs {
		payload[key] = value
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		l.base.Printf("{\"ts\":\"%s\",\"level\":\"error\",\"msg\":\"failed to encode log line\",\"error\":%q}\n", time.Now().UTC().Format(time.RFC3339), err.Error())
		return
	}

	l.base.Println(string(encoded))
}

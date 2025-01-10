package collectors

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ollystack/ollystack/agents/universal-agent/internal/config"
	"github.com/ollystack/ollystack/agents/universal-agent/internal/exporters"
	"go.uber.org/zap"
)

// LogRecord represents a single log record.
type LogRecord struct {
	Timestamp  time.Time
	Body       string
	Severity   string
	Attributes map[string]string
	Resource   map[string]string
}

// LogsCollector collects logs from various sources.
type LogsCollector struct {
	cfg      config.LogsConfig
	exporter *exporters.LogExporter
	logger   *zap.Logger

	watchers map[string]*fileWatcher
	mu       sync.RWMutex
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// fileWatcher watches a single file for new log lines.
type fileWatcher struct {
	path     string
	file     *os.File
	offset   int64
	logger   *zap.Logger
	exporter *exporters.LogExporter
}

// NewLogsCollector creates a new LogsCollector.
func NewLogsCollector(cfg config.LogsConfig, exporter *exporters.LogExporter, logger *zap.Logger) (*LogsCollector, error) {
	return &LogsCollector{
		cfg:      cfg,
		exporter: exporter,
		logger:   logger,
		watchers: make(map[string]*fileWatcher),
		stopChan: make(chan struct{}),
	}, nil
}

// Start starts the log collection.
func (c *LogsCollector) Start(ctx context.Context) error {
	c.logger.Info("Starting logs collector")

	// Expand glob patterns
	var files []string
	for _, pattern := range c.cfg.Paths {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			c.logger.Warn("Failed to expand glob pattern",
				zap.String("pattern", pattern),
				zap.Error(err))
			continue
		}
		files = append(files, matches...)
	}

	// Start watching each file
	for _, file := range files {
		if err := c.watchFile(ctx, file); err != nil {
			c.logger.Warn("Failed to watch file",
				zap.String("file", file),
				zap.Error(err))
		}
	}

	// Start file discovery goroutine for new files
	c.wg.Add(1)
	go c.discoverFiles(ctx)

	c.logger.Info("Logs collector started",
		zap.Int("watched_files", len(c.watchers)))

	return nil
}

// Stop stops the log collection.
func (c *LogsCollector) Stop() error {
	c.logger.Info("Stopping logs collector")

	close(c.stopChan)
	c.wg.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, watcher := range c.watchers {
		if watcher.file != nil {
			watcher.file.Close()
		}
	}
	c.watchers = make(map[string]*fileWatcher)

	c.logger.Info("Logs collector stopped")
	return nil
}

// watchFile starts watching a file for new log lines.
func (c *LogsCollector) watchFile(ctx context.Context, path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already watching
	if _, exists := c.watchers[path]; exists {
		return nil
	}

	// Open the file
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}

	// Seek to end of file
	offset, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		file.Close()
		return fmt.Errorf("failed to seek to end: %w", err)
	}

	watcher := &fileWatcher{
		path:     path,
		file:     file,
		offset:   offset,
		logger:   c.logger,
		exporter: c.exporter,
	}

	c.watchers[path] = watcher

	// Start watching goroutine
	c.wg.Add(1)
	go c.watchLoop(ctx, watcher)

	c.logger.Debug("Started watching file", zap.String("path", path))

	return nil
}

// watchLoop continuously watches a file for new lines.
func (c *LogsCollector) watchLoop(ctx context.Context, watcher *fileWatcher) {
	defer c.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	reader := bufio.NewReader(watcher.file)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.readNewLines(ctx, watcher, reader)
		}
	}
}

// readNewLines reads new lines from a file.
func (c *LogsCollector) readNewLines(ctx context.Context, watcher *fileWatcher, reader *bufio.Reader) {
	// Check if file was rotated
	stat, err := os.Stat(watcher.path)
	if err != nil {
		c.logger.Debug("File not accessible",
			zap.String("path", watcher.path),
			zap.Error(err))
		return
	}

	// If file was truncated or rotated, reopen
	if stat.Size() < watcher.offset {
		c.logger.Debug("File was rotated, reopening",
			zap.String("path", watcher.path))

		watcher.file.Close()
		file, err := os.Open(watcher.path)
		if err != nil {
			c.logger.Warn("Failed to reopen file",
				zap.String("path", watcher.path),
				zap.Error(err))
			return
		}
		watcher.file = file
		watcher.offset = 0
		reader.Reset(file)
	}

	// Read new lines
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				c.logger.Warn("Error reading file",
					zap.String("path", watcher.path),
					zap.Error(err))
			}
			break
		}

		if len(line) > 0 {
			record := LogRecord{
				Timestamp: time.Now(),
				Body:      line,
				Severity:  c.detectSeverity(line),
				Attributes: map[string]string{
					"log.file.path": watcher.path,
					"log.file.name": filepath.Base(watcher.path),
				},
			}

			if err := watcher.exporter.Export(ctx, record); err != nil {
				c.logger.Warn("Failed to export log record",
					zap.String("path", watcher.path),
					zap.Error(err))
			}
		}

		watcher.offset += int64(len(line))
	}
}

// detectSeverity attempts to detect the log severity from the line.
func (c *LogsCollector) detectSeverity(line string) string {
	// Simple keyword-based detection
	keywords := map[string]string{
		"FATAL": "FATAL",
		"fatal": "FATAL",
		"ERROR": "ERROR",
		"error": "ERROR",
		"ERR":   "ERROR",
		"WARN":  "WARN",
		"warn":  "WARN",
		"WARNING": "WARN",
		"warning": "WARN",
		"INFO":  "INFO",
		"info":  "INFO",
		"DEBUG": "DEBUG",
		"debug": "DEBUG",
		"TRACE": "TRACE",
		"trace": "TRACE",
	}

	for keyword, severity := range keywords {
		if containsKeyword(line, keyword) {
			return severity
		}
	}

	return "INFO"
}

// containsKeyword checks if a line contains a keyword.
func containsKeyword(line, keyword string) bool {
	// Simple substring search
	// Could be improved with regex for word boundaries
	for i := 0; i <= len(line)-len(keyword); i++ {
		if line[i:i+len(keyword)] == keyword {
			return true
		}
	}
	return false
}

// discoverFiles periodically discovers new files matching the patterns.
func (c *LogsCollector) discoverFiles(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopChan:
			return
		case <-ticker.C:
			for _, pattern := range c.cfg.Paths {
				matches, err := filepath.Glob(pattern)
				if err != nil {
					continue
				}
				for _, file := range matches {
					c.watchFile(ctx, file)
				}
			}
		}
	}
}

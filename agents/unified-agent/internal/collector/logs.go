package collector

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"

	"github.com/ollystack/unified-agent/internal/config"
	"github.com/ollystack/unified-agent/internal/pipeline"
	"github.com/ollystack/unified-agent/internal/types"
)

// LogsConfig configures the logs collector
type LogsConfig struct {
	Sources              []config.LogSource
	DeduplicationEnabled bool
	DeduplicationWindow  time.Duration
	MaxPatterns          int
	MultilinePattern     string
	MultilineMaxLines    int
}

// LogsCollector collects logs efficiently with deduplication
type LogsCollector struct {
	config   LogsConfig
	pipeline *pipeline.Pipeline
	logger   *zap.Logger

	// File watchers
	watcher *fsnotify.Watcher

	// File readers
	mu      sync.RWMutex
	readers map[string]*logReader

	// Pattern deduplication
	deduper *logDeduplicator

	// Multiline pattern
	multilineRe *regexp.Regexp
}

// logReader tracks reading position for a log file
type logReader struct {
	path     string
	file     *os.File
	offset   int64
	service  string
	parseJSON bool
	extractTrace bool
}

// logDeduplicator reduces duplicate log messages
type logDeduplicator struct {
	mu       sync.RWMutex
	patterns map[string]*patternInfo
	window   time.Duration
	maxSize  int
}

type patternInfo struct {
	template  string
	count     int64
	firstSeen time.Time
	lastSeen  time.Time
	sample    string
}

// NewLogsCollector creates a new logs collector
func NewLogsCollector(cfg LogsConfig, p *pipeline.Pipeline, logger *zap.Logger) (*LogsCollector, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	c := &LogsCollector{
		config:   cfg,
		pipeline: p,
		logger:   logger,
		watcher:  watcher,
		readers:  make(map[string]*logReader),
	}

	// Initialize deduplicator
	if cfg.DeduplicationEnabled {
		c.deduper = &logDeduplicator{
			patterns: make(map[string]*patternInfo),
			window:   cfg.DeduplicationWindow,
			maxSize:  cfg.MaxPatterns,
		}
	}

	// Compile multiline pattern
	if cfg.MultilinePattern != "" {
		re, err := regexp.Compile(cfg.MultilinePattern)
		if err != nil {
			logger.Warn("Invalid multiline pattern", zap.Error(err))
		} else {
			c.multilineRe = re
		}
	}

	return c, nil
}

// Start begins log collection
func (c *LogsCollector) Start(ctx context.Context) error {
	// Initialize sources
	for _, source := range c.config.Sources {
		if err := c.initSource(source); err != nil {
			c.logger.Error("Failed to init log source",
				zap.String("type", source.Type),
				zap.String("path", source.Path),
				zap.Error(err))
		}
	}

	// Start deduplication flusher
	if c.deduper != nil {
		go c.flushDeduplication(ctx)
	}

	// Watch for file changes
	go c.watchLoop(ctx)

	// Read existing files
	go c.readLoop(ctx)

	<-ctx.Done()

	// Cleanup
	c.watcher.Close()
	c.mu.Lock()
	for _, r := range c.readers {
		if r.file != nil {
			r.file.Close()
		}
	}
	c.mu.Unlock()

	return nil
}

func (c *LogsCollector) initSource(source config.LogSource) error {
	switch source.Type {
	case "file":
		return c.initFileSource(source)
	case "journald":
		return c.initJournaldSource(source)
	case "docker":
		return c.initDockerSource(source)
	default:
		c.logger.Warn("Unknown log source type", zap.String("type", source.Type))
		return nil
	}
}

func (c *LogsCollector) initFileSource(source config.LogSource) error {
	// Expand glob pattern
	matches, err := filepath.Glob(source.Path)
	if err != nil {
		return err
	}

	for _, path := range matches {
		// Watch directory for new files
		dir := filepath.Dir(path)
		c.watcher.Add(dir)

		// Open file
		if err := c.openLogFile(path, source); err != nil {
			c.logger.Warn("Failed to open log file", zap.String("path", path), zap.Error(err))
		}
	}

	return nil
}

func (c *LogsCollector) openLogFile(path string, source config.LogSource) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.readers[path]; exists {
		return nil // Already tracking
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}

	// Seek to end for new files (don't read historical)
	offset, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		file.Close()
		return err
	}

	c.readers[path] = &logReader{
		path:         path,
		file:         file,
		offset:       offset,
		service:      source.Service,
		parseJSON:    source.ParseJSON,
		extractTrace: source.ExtractTraceContext,
	}

	c.logger.Info("Started watching log file", zap.String("path", path))
	return nil
}

func (c *LogsCollector) initJournaldSource(source config.LogSource) error {
	// TODO: Implement journald support using coreos/go-systemd
	c.logger.Info("Journald source not yet implemented")
	return nil
}

func (c *LogsCollector) initDockerSource(source config.LogSource) error {
	// TODO: Implement Docker log streaming
	c.logger.Info("Docker log source not yet implemented")
	return nil
}

func (c *LogsCollector) watchLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-c.watcher.Events:
			if !ok {
				return
			}

			if event.Op&fsnotify.Create == fsnotify.Create {
				// Check if this matches any of our patterns
				for _, source := range c.config.Sources {
					if source.Type == "file" {
						matched, _ := filepath.Match(source.Path, event.Name)
						if matched {
							c.openLogFile(event.Name, source)
						}
					}
				}
			}

		case err, ok := <-c.watcher.Errors:
			if !ok {
				return
			}
			c.logger.Error("Watcher error", zap.Error(err))
		}
	}
}

func (c *LogsCollector) readLoop(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond) // Poll interval
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.readAllFiles()
		}
	}
}

func (c *LogsCollector) readAllFiles() {
	c.mu.RLock()
	readers := make([]*logReader, 0, len(c.readers))
	for _, r := range c.readers {
		readers = append(readers, r)
	}
	c.mu.RUnlock()

	for _, reader := range readers {
		c.readFile(reader)
	}
}

func (c *LogsCollector) readFile(reader *logReader) {
	// Check if file still exists
	info, err := reader.file.Stat()
	if err != nil {
		c.logger.Debug("Log file stat error", zap.String("path", reader.path), zap.Error(err))
		return
	}

	// Handle truncation (log rotation)
	if info.Size() < reader.offset {
		reader.offset = 0
		reader.file.Seek(0, io.SeekStart)
	}

	// Read new lines
	scanner := bufio.NewScanner(reader.file)
	lineCount := 0
	maxLinesPerRead := 1000 // Prevent blocking

	for scanner.Scan() && lineCount < maxLinesPerRead {
		line := scanner.Text()
		if line == "" {
			continue
		}

		c.processLogLine(reader, line, time.Now())
		lineCount++
	}

	// Update offset
	newOffset, _ := reader.file.Seek(0, io.SeekCurrent)
	reader.offset = newOffset
}

func (c *LogsCollector) processLogLine(reader *logReader, line string, timestamp time.Time) {
	// Create log record
	logRecord := types.LogRecord{
		Timestamp: timestamp,
		Body:      line,
		Attributes: map[string]string{
			"source": reader.path,
		},
	}

	// Set service if configured
	if reader.service != "" {
		logRecord.Service = reader.service
	}

	// Extract trace context if enabled
	if reader.extractTrace {
		c.extractTraceContext(&logRecord, line)
	}

	// Detect severity
	logRecord.Severity = c.detectSeverity(line)

	// Deduplicate if enabled
	if c.deduper != nil {
		if !c.deduper.add(line, timestamp) {
			return // Duplicate within window
		}
	}

	// Send to pipeline
	c.pipeline.ProcessLog(logRecord)
}

func (c *LogsCollector) extractTraceContext(log *types.LogRecord, line string) {
	// Common trace ID patterns
	tracePatterns := []*regexp.Regexp{
		regexp.MustCompile(`trace[_-]?id[=:]["']?([a-f0-9]{32})`),
		regexp.MustCompile(`traceid[=:]["']?([a-f0-9]{32})`),
		regexp.MustCompile(`x-b3-traceid[=:]["']?([a-f0-9]{32})`),
	}

	spanPatterns := []*regexp.Regexp{
		regexp.MustCompile(`span[_-]?id[=:]["']?([a-f0-9]{16})`),
		regexp.MustCompile(`spanid[=:]["']?([a-f0-9]{16})`),
	}

	lineLower := strings.ToLower(line)

	for _, re := range tracePatterns {
		if matches := re.FindStringSubmatch(lineLower); len(matches) > 1 {
			log.TraceID = matches[1]
			break
		}
	}

	for _, re := range spanPatterns {
		if matches := re.FindStringSubmatch(lineLower); len(matches) > 1 {
			log.SpanID = matches[1]
			break
		}
	}
}

func (c *LogsCollector) detectSeverity(line string) types.Severity {
	lineLower := strings.ToLower(line)

	// Check common patterns
	switch {
	case strings.Contains(lineLower, "fatal") || strings.Contains(lineLower, "panic"):
		return types.SeverityFatal
	case strings.Contains(lineLower, "error") || strings.Contains(lineLower, "err]"):
		return types.SeverityError
	case strings.Contains(lineLower, "warn"):
		return types.SeverityWarn
	case strings.Contains(lineLower, "info"):
		return types.SeverityInfo
	case strings.Contains(lineLower, "debug") || strings.Contains(lineLower, "trace"):
		return types.SeverityDebug
	default:
		return types.SeverityInfo
	}
}

func (c *LogsCollector) flushDeduplication(ctx context.Context) {
	ticker := time.NewTicker(c.deduper.window)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.deduper.flush(c.pipeline)
		}
	}
}

// logDeduplicator methods

func (d *logDeduplicator) add(line string, timestamp time.Time) bool {
	template := d.extractTemplate(line)
	hash := d.hash(template)

	d.mu.Lock()
	defer d.mu.Unlock()

	if info, exists := d.patterns[hash]; exists {
		info.count++
		info.lastSeen = timestamp
		return false // Duplicate
	}

	// Evict old patterns if at capacity
	if len(d.patterns) >= d.maxSize {
		d.evictOldest()
	}

	d.patterns[hash] = &patternInfo{
		template:  template,
		count:     1,
		firstSeen: timestamp,
		lastSeen:  timestamp,
		sample:    line,
	}

	return true // New pattern
}

func (d *logDeduplicator) flush(p *pipeline.Pipeline) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	for hash, info := range d.patterns {
		// Only flush patterns within window
		if now.Sub(info.lastSeen) > d.window {
			delete(d.patterns, hash)
			continue
		}

		// If count > 1, send aggregated log
		if info.count > 1 {
			p.ProcessLog(types.LogRecord{
				Timestamp: info.lastSeen,
				Body:      info.sample,
				Severity:  types.SeverityInfo,
				Attributes: map[string]string{
					"deduplicated":     "true",
					"occurrence_count": string(rune(info.count)),
					"pattern_template": info.template,
				},
			})
		}

		// Reset count
		info.count = 0
	}
}

func (d *logDeduplicator) extractTemplate(line string) string {
	// Replace variable parts with placeholders
	// This is a simplified version of log parsing algorithms like Drain

	// Replace numbers
	template := regexp.MustCompile(`\b\d+\b`).ReplaceAllString(line, "<NUM>")

	// Replace UUIDs
	template = regexp.MustCompile(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`).ReplaceAllString(template, "<UUID>")

	// Replace IPs
	template = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`).ReplaceAllString(template, "<IP>")

	// Replace hex strings
	template = regexp.MustCompile(`\b[a-f0-9]{16,}\b`).ReplaceAllString(template, "<HEX>")

	// Replace paths
	template = regexp.MustCompile(`/[^\s]+`).ReplaceAllString(template, "<PATH>")

	// Replace timestamps
	template = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}`).ReplaceAllString(template, "<TIMESTAMP>")

	return template
}

func (d *logDeduplicator) hash(template string) string {
	h := md5.Sum([]byte(template))
	return hex.EncodeToString(h[:])
}

func (d *logDeduplicator) evictOldest() {
	var oldestHash string
	var oldestTime time.Time

	for hash, info := range d.patterns {
		if oldestHash == "" || info.lastSeen.Before(oldestTime) {
			oldestHash = hash
			oldestTime = info.lastSeen
		}
	}

	if oldestHash != "" {
		delete(d.patterns, oldestHash)
	}
}

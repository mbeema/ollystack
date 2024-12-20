// Package collector implements efficient data collectors
package collector

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
	"go.uber.org/zap"

	"github.com/ollystack/unified-agent/internal/pipeline"
	"github.com/ollystack/unified-agent/internal/types"
)

// MetricsConfig configures the metrics collector
type MetricsConfig struct {
	Interval         time.Duration
	CollectCPU       bool
	CollectMemory    bool
	CollectDisk      bool
	CollectNetwork   bool
	CollectFS        bool
	CollectProcess   bool
	CollectContainer bool
	ProcessInclude   []string
	ProcessExclude   []string
	MaxProcesses     int
	DockerSocket     string
}

// MetricsCollector collects host metrics efficiently
type MetricsCollector struct {
	config   MetricsConfig
	pipeline *pipeline.Pipeline
	logger   *zap.Logger

	// Cached values for delta calculations
	mu             sync.RWMutex
	lastCPUTimes   []cpu.TimesStat
	lastNetIO      map[string]net.IOCountersStat
	lastDiskIO     map[string]disk.IOCountersStat
	lastCollectTime time.Time

	// Host info (collected once)
	hostInfo *host.InfoStat
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(cfg MetricsConfig, p *pipeline.Pipeline, logger *zap.Logger) (*MetricsCollector, error) {
	// Get host info once at startup
	hostInfo, err := host.Info()
	if err != nil {
		logger.Warn("Failed to get host info", zap.Error(err))
	}

	return &MetricsCollector{
		config:      cfg,
		pipeline:    p,
		logger:      logger,
		lastNetIO:   make(map[string]net.IOCountersStat),
		lastDiskIO:  make(map[string]disk.IOCountersStat),
		hostInfo:    hostInfo,
	}, nil
}

// Start begins metric collection
func (c *MetricsCollector) Start(ctx context.Context) error {
	ticker := time.NewTicker(c.config.Interval)
	defer ticker.Stop()

	// Initial collection
	c.collect()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c.collect()
		}
	}
}

func (c *MetricsCollector) collect() {
	now := time.Now()
	metrics := make([]types.Metric, 0, 50)

	// Collect enabled metrics
	if c.config.CollectCPU {
		metrics = append(metrics, c.collectCPU(now)...)
	}
	if c.config.CollectMemory {
		metrics = append(metrics, c.collectMemory(now)...)
	}
	if c.config.CollectDisk {
		metrics = append(metrics, c.collectDisk(now)...)
	}
	if c.config.CollectNetwork {
		metrics = append(metrics, c.collectNetwork(now)...)
	}
	if c.config.CollectFS {
		metrics = append(metrics, c.collectFilesystem(now)...)
	}
	if c.config.CollectProcess {
		metrics = append(metrics, c.collectProcess(now)...)
	}

	// Send to pipeline
	for _, m := range metrics {
		c.pipeline.ProcessMetric(m)
	}

	c.mu.Lock()
	c.lastCollectTime = now
	c.mu.Unlock()
}

func (c *MetricsCollector) collectCPU(now time.Time) []types.Metric {
	metrics := make([]types.Metric, 0, 10)

	// CPU times per-CPU (for utilization calculation)
	cpuTimes, err := cpu.Times(true)
	if err != nil {
		c.logger.Debug("Failed to get CPU times", zap.Error(err))
		return metrics
	}

	c.mu.Lock()
	lastCPU := c.lastCPUTimes
	c.lastCPUTimes = cpuTimes
	c.mu.Unlock()

	// Calculate utilization if we have previous values
	if lastCPU != nil && len(lastCPU) == len(cpuTimes) {
		for i, curr := range cpuTimes {
			prev := lastCPU[i]

			totalDelta := (curr.User + curr.System + curr.Idle + curr.Nice + curr.Iowait + curr.Irq + curr.Softirq + curr.Steal) -
				(prev.User + prev.System + prev.Idle + prev.Nice + prev.Iowait + prev.Irq + prev.Softirq + prev.Steal)

			if totalDelta > 0 {
				userPct := ((curr.User - prev.User) / totalDelta) * 100
				systemPct := ((curr.System - prev.System) / totalDelta) * 100
				idlePct := ((curr.Idle - prev.Idle) / totalDelta) * 100
				iowaitPct := ((curr.Iowait - prev.Iowait) / totalDelta) * 100

				cpuLabels := map[string]string{"cpu": curr.CPU}

				metrics = append(metrics,
					types.Metric{
						Name:      "system.cpu.user",
						Value:     userPct,
						Timestamp: now,
						Labels:    cpuLabels,
						Type:      types.MetricTypeGauge,
					},
					types.Metric{
						Name:      "system.cpu.system",
						Value:     systemPct,
						Timestamp: now,
						Labels:    cpuLabels,
						Type:      types.MetricTypeGauge,
					},
					types.Metric{
						Name:      "system.cpu.idle",
						Value:     idlePct,
						Timestamp: now,
						Labels:    cpuLabels,
						Type:      types.MetricTypeGauge,
					},
					types.Metric{
						Name:      "system.cpu.iowait",
						Value:     iowaitPct,
						Timestamp: now,
						Labels:    cpuLabels,
						Type:      types.MetricTypeGauge,
					},
				)
			}
		}
	}

	// Load average (cheap, always collect)
	loadAvg, err := cpu.Percent(0, false)
	if err == nil && len(loadAvg) > 0 {
		metrics = append(metrics, types.Metric{
			Name:      "system.cpu.utilization",
			Value:     loadAvg[0],
			Timestamp: now,
			Type:      types.MetricTypeGauge,
		})
	}

	return metrics
}

func (c *MetricsCollector) collectMemory(now time.Time) []types.Metric {
	metrics := make([]types.Metric, 0, 10)

	vm, err := mem.VirtualMemory()
	if err != nil {
		c.logger.Debug("Failed to get memory stats", zap.Error(err))
		return metrics
	}

	metrics = append(metrics,
		types.Metric{
			Name:      "system.memory.total",
			Value:     float64(vm.Total),
			Timestamp: now,
			Type:      types.MetricTypeGauge,
			Unit:      "bytes",
		},
		types.Metric{
			Name:      "system.memory.used",
			Value:     float64(vm.Used),
			Timestamp: now,
			Type:      types.MetricTypeGauge,
			Unit:      "bytes",
		},
		types.Metric{
			Name:      "system.memory.available",
			Value:     float64(vm.Available),
			Timestamp: now,
			Type:      types.MetricTypeGauge,
			Unit:      "bytes",
		},
		types.Metric{
			Name:      "system.memory.utilization",
			Value:     vm.UsedPercent,
			Timestamp: now,
			Type:      types.MetricTypeGauge,
			Unit:      "percent",
		},
	)

	// Swap memory
	swap, err := mem.SwapMemory()
	if err == nil {
		metrics = append(metrics,
			types.Metric{
				Name:      "system.swap.total",
				Value:     float64(swap.Total),
				Timestamp: now,
				Type:      types.MetricTypeGauge,
				Unit:      "bytes",
			},
			types.Metric{
				Name:      "system.swap.used",
				Value:     float64(swap.Used),
				Timestamp: now,
				Type:      types.MetricTypeGauge,
				Unit:      "bytes",
			},
		)
	}

	return metrics
}

func (c *MetricsCollector) collectDisk(now time.Time) []types.Metric {
	metrics := make([]types.Metric, 0, 20)

	// Disk I/O counters
	diskIO, err := disk.IOCounters()
	if err != nil {
		c.logger.Debug("Failed to get disk IO", zap.Error(err))
		return metrics
	}

	c.mu.Lock()
	lastDiskIO := c.lastDiskIO
	c.lastDiskIO = diskIO
	c.mu.Unlock()

	for name, io := range diskIO {
		labels := map[string]string{"device": name}

		metrics = append(metrics,
			types.Metric{
				Name:      "system.disk.read_bytes",
				Value:     float64(io.ReadBytes),
				Timestamp: now,
				Labels:    labels,
				Type:      types.MetricTypeCounter,
				Unit:      "bytes",
			},
			types.Metric{
				Name:      "system.disk.write_bytes",
				Value:     float64(io.WriteBytes),
				Timestamp: now,
				Labels:    labels,
				Type:      types.MetricTypeCounter,
				Unit:      "bytes",
			},
		)

		// Calculate rates if we have previous values
		if prev, ok := lastDiskIO[name]; ok {
			c.mu.RLock()
			timeDelta := now.Sub(c.lastCollectTime).Seconds()
			c.mu.RUnlock()

			if timeDelta > 0 {
				readRate := float64(io.ReadBytes-prev.ReadBytes) / timeDelta
				writeRate := float64(io.WriteBytes-prev.WriteBytes) / timeDelta

				metrics = append(metrics,
					types.Metric{
						Name:      "system.disk.read_bytes_rate",
						Value:     readRate,
						Timestamp: now,
						Labels:    labels,
						Type:      types.MetricTypeGauge,
						Unit:      "bytes/s",
					},
					types.Metric{
						Name:      "system.disk.write_bytes_rate",
						Value:     writeRate,
						Timestamp: now,
						Labels:    labels,
						Type:      types.MetricTypeGauge,
						Unit:      "bytes/s",
					},
				)
			}
		}
	}

	return metrics
}

func (c *MetricsCollector) collectNetwork(now time.Time) []types.Metric {
	metrics := make([]types.Metric, 0, 20)

	netIO, err := net.IOCounters(true)
	if err != nil {
		c.logger.Debug("Failed to get network IO", zap.Error(err))
		return metrics
	}

	c.mu.Lock()
	lastNetIO := c.lastNetIO
	newNetIO := make(map[string]net.IOCountersStat)
	for _, io := range netIO {
		newNetIO[io.Name] = io
	}
	c.lastNetIO = newNetIO
	c.mu.Unlock()

	for _, io := range netIO {
		// Skip loopback
		if io.Name == "lo" || io.Name == "lo0" {
			continue
		}

		labels := map[string]string{"interface": io.Name}

		metrics = append(metrics,
			types.Metric{
				Name:      "system.network.bytes_recv",
				Value:     float64(io.BytesRecv),
				Timestamp: now,
				Labels:    labels,
				Type:      types.MetricTypeCounter,
				Unit:      "bytes",
			},
			types.Metric{
				Name:      "system.network.bytes_sent",
				Value:     float64(io.BytesSent),
				Timestamp: now,
				Labels:    labels,
				Type:      types.MetricTypeCounter,
				Unit:      "bytes",
			},
			types.Metric{
				Name:      "system.network.packets_recv",
				Value:     float64(io.PacketsRecv),
				Timestamp: now,
				Labels:    labels,
				Type:      types.MetricTypeCounter,
			},
			types.Metric{
				Name:      "system.network.packets_sent",
				Value:     float64(io.PacketsSent),
				Timestamp: now,
				Labels:    labels,
				Type:      types.MetricTypeCounter,
			},
			types.Metric{
				Name:      "system.network.errors_in",
				Value:     float64(io.Errin),
				Timestamp: now,
				Labels:    labels,
				Type:      types.MetricTypeCounter,
			},
			types.Metric{
				Name:      "system.network.errors_out",
				Value:     float64(io.Errout),
				Timestamp: now,
				Labels:    labels,
				Type:      types.MetricTypeCounter,
			},
		)

		// Calculate rates
		if prev, ok := lastNetIO[io.Name]; ok {
			c.mu.RLock()
			timeDelta := now.Sub(c.lastCollectTime).Seconds()
			c.mu.RUnlock()

			if timeDelta > 0 {
				recvRate := float64(io.BytesRecv-prev.BytesRecv) / timeDelta
				sentRate := float64(io.BytesSent-prev.BytesSent) / timeDelta

				metrics = append(metrics,
					types.Metric{
						Name:      "system.network.bytes_recv_rate",
						Value:     recvRate,
						Timestamp: now,
						Labels:    labels,
						Type:      types.MetricTypeGauge,
						Unit:      "bytes/s",
					},
					types.Metric{
						Name:      "system.network.bytes_sent_rate",
						Value:     sentRate,
						Timestamp: now,
						Labels:    labels,
						Type:      types.MetricTypeGauge,
						Unit:      "bytes/s",
					},
				)
			}
		}
	}

	return metrics
}

func (c *MetricsCollector) collectFilesystem(now time.Time) []types.Metric {
	metrics := make([]types.Metric, 0, 20)

	partitions, err := disk.Partitions(false) // false = physical only
	if err != nil {
		c.logger.Debug("Failed to get partitions", zap.Error(err))
		return metrics
	}

	for _, p := range partitions {
		// Skip certain filesystem types
		switch p.Fstype {
		case "squashfs", "tmpfs", "devtmpfs", "overlay":
			continue
		}

		usage, err := disk.Usage(p.Mountpoint)
		if err != nil {
			continue
		}

		labels := map[string]string{
			"device":     p.Device,
			"mountpoint": p.Mountpoint,
			"fstype":     p.Fstype,
		}

		metrics = append(metrics,
			types.Metric{
				Name:      "system.filesystem.total",
				Value:     float64(usage.Total),
				Timestamp: now,
				Labels:    labels,
				Type:      types.MetricTypeGauge,
				Unit:      "bytes",
			},
			types.Metric{
				Name:      "system.filesystem.used",
				Value:     float64(usage.Used),
				Timestamp: now,
				Labels:    labels,
				Type:      types.MetricTypeGauge,
				Unit:      "bytes",
			},
			types.Metric{
				Name:      "system.filesystem.free",
				Value:     float64(usage.Free),
				Timestamp: now,
				Labels:    labels,
				Type:      types.MetricTypeGauge,
				Unit:      "bytes",
			},
			types.Metric{
				Name:      "system.filesystem.utilization",
				Value:     usage.UsedPercent,
				Timestamp: now,
				Labels:    labels,
				Type:      types.MetricTypeGauge,
				Unit:      "percent",
			},
			types.Metric{
				Name:      "system.filesystem.inodes_total",
				Value:     float64(usage.InodesTotal),
				Timestamp: now,
				Labels:    labels,
				Type:      types.MetricTypeGauge,
			},
			types.Metric{
				Name:      "system.filesystem.inodes_used",
				Value:     float64(usage.InodesUsed),
				Timestamp: now,
				Labels:    labels,
				Type:      types.MetricTypeGauge,
			},
		)
	}

	return metrics
}

func (c *MetricsCollector) collectProcess(now time.Time) []types.Metric {
	metrics := make([]types.Metric, 0, 100)

	procs, err := process.Processes()
	if err != nil {
		c.logger.Debug("Failed to get processes", zap.Error(err))
		return metrics
	}

	// Limit number of processes
	limit := c.config.MaxProcesses
	if limit == 0 || limit > len(procs) {
		limit = len(procs)
	}

	// Sort by CPU or memory to get top processes
	// For now, just take first N
	count := 0
	for _, p := range procs {
		if count >= limit {
			break
		}

		name, err := p.Name()
		if err != nil {
			continue
		}

		// Check include/exclude patterns
		if !c.shouldTrackProcess(name) {
			continue
		}

		cpuPercent, _ := p.CPUPercent()
		memInfo, _ := p.MemoryInfo()

		labels := map[string]string{
			"pid":  fmt.Sprintf("%d", p.Pid),
			"name": name,
		}

		metrics = append(metrics,
			types.Metric{
				Name:      "system.process.cpu",
				Value:     cpuPercent,
				Timestamp: now,
				Labels:    labels,
				Type:      types.MetricTypeGauge,
				Unit:      "percent",
			},
		)

		if memInfo != nil {
			metrics = append(metrics,
				types.Metric{
					Name:      "system.process.memory_rss",
					Value:     float64(memInfo.RSS),
					Timestamp: now,
					Labels:    labels,
					Type:      types.MetricTypeGauge,
					Unit:      "bytes",
				},
			)
		}

		count++
	}

	// Agent self-metrics
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	metrics = append(metrics,
		types.Metric{
			Name:      "ollystack.agent.memory_alloc",
			Value:     float64(memStats.Alloc),
			Timestamp: now,
			Type:      types.MetricTypeGauge,
			Unit:      "bytes",
		},
		types.Metric{
			Name:      "ollystack.agent.goroutines",
			Value:     float64(runtime.NumGoroutine()),
			Timestamp: now,
			Type:      types.MetricTypeGauge,
		},
	)

	return metrics
}

func (c *MetricsCollector) shouldTrackProcess(name string) bool {
	// Check exclude patterns first
	for _, pattern := range c.config.ProcessExclude {
		if matchPattern(name, pattern) {
			return false
		}
	}

	// If include patterns specified, must match one
	if len(c.config.ProcessInclude) > 0 {
		for _, pattern := range c.config.ProcessInclude {
			if matchPattern(name, pattern) {
				return true
			}
		}
		return false
	}

	return true
}

// Simple glob matching
func matchPattern(name, pattern string) bool {
	if pattern == "*" {
		return true
	}
	// TODO: implement proper glob matching
	return name == pattern
}

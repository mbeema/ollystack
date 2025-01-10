// Package collectors provides data collectors for metrics, logs, and traces.
package collectors

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/ollystack/ollystack/agents/universal-agent/internal/config"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

// MetricsCollector collects system metrics.
type MetricsCollector struct {
	cfg    config.MetricsConfig
	meter  metric.Meter
	logger *zap.Logger

	// Metric instruments
	cpuUsage        metric.Float64Gauge
	cpuUser         metric.Float64Gauge
	cpuSystem       metric.Float64Gauge
	cpuIdle         metric.Float64Gauge
	cpuIowait       metric.Float64Gauge
	memoryTotal     metric.Int64Gauge
	memoryUsed      metric.Int64Gauge
	memoryFree      metric.Int64Gauge
	memoryAvailable metric.Int64Gauge
	memoryCached    metric.Int64Gauge
	memoryBuffers   metric.Int64Gauge
	swapTotal       metric.Int64Gauge
	swapUsed        metric.Int64Gauge
	swapFree        metric.Int64Gauge
	diskTotal       metric.Int64Gauge
	diskUsed        metric.Int64Gauge
	diskFree        metric.Int64Gauge
	diskUsedPercent metric.Float64Gauge
	diskReadBytes   metric.Int64Counter
	diskWriteBytes  metric.Int64Counter
	diskReadOps     metric.Int64Counter
	diskWriteOps    metric.Int64Counter
	netBytesRecv    metric.Int64Counter
	netBytesSent    metric.Int64Counter
	netPacketsRecv  metric.Int64Counter
	netPacketsSent  metric.Int64Counter
	netErrsIn       metric.Int64Counter
	netErrsOut      metric.Int64Counter
	loadAvg1        metric.Float64Gauge
	loadAvg5        metric.Float64Gauge
	loadAvg15       metric.Float64Gauge
	processCount    metric.Int64Gauge
	uptime          metric.Int64Gauge

	// Previous values for delta calculations
	prevNetStats map[string]net.IOCountersStat
	prevDiskIO   map[string]disk.IOCountersStat
	mu           sync.Mutex
}

// NewMetricsCollector creates a new MetricsCollector.
func NewMetricsCollector(cfg config.MetricsConfig, meter metric.Meter, logger *zap.Logger) (*MetricsCollector, error) {
	c := &MetricsCollector{
		cfg:          cfg,
		meter:        meter,
		logger:       logger,
		prevNetStats: make(map[string]net.IOCountersStat),
		prevDiskIO:   make(map[string]disk.IOCountersStat),
	}

	if err := c.initMetrics(); err != nil {
		return nil, fmt.Errorf("failed to initialize metrics: %w", err)
	}

	return c, nil
}

// initMetrics initializes all metric instruments.
func (c *MetricsCollector) initMetrics() error {
	var err error

	// CPU metrics
	if c.cfg.Collectors.CPU {
		c.cpuUsage, err = c.meter.Float64Gauge("system.cpu.usage",
			metric.WithDescription("CPU usage percentage"),
			metric.WithUnit("%"))
		if err != nil {
			return err
		}

		c.cpuUser, err = c.meter.Float64Gauge("system.cpu.user",
			metric.WithDescription("CPU user time percentage"),
			metric.WithUnit("%"))
		if err != nil {
			return err
		}

		c.cpuSystem, err = c.meter.Float64Gauge("system.cpu.system",
			metric.WithDescription("CPU system time percentage"),
			metric.WithUnit("%"))
		if err != nil {
			return err
		}

		c.cpuIdle, err = c.meter.Float64Gauge("system.cpu.idle",
			metric.WithDescription("CPU idle time percentage"),
			metric.WithUnit("%"))
		if err != nil {
			return err
		}
	}

	// Memory metrics
	if c.cfg.Collectors.Memory {
		c.memoryTotal, err = c.meter.Int64Gauge("system.memory.total",
			metric.WithDescription("Total memory in bytes"),
			metric.WithUnit("By"))
		if err != nil {
			return err
		}

		c.memoryUsed, err = c.meter.Int64Gauge("system.memory.used",
			metric.WithDescription("Used memory in bytes"),
			metric.WithUnit("By"))
		if err != nil {
			return err
		}

		c.memoryFree, err = c.meter.Int64Gauge("system.memory.free",
			metric.WithDescription("Free memory in bytes"),
			metric.WithUnit("By"))
		if err != nil {
			return err
		}

		c.memoryAvailable, err = c.meter.Int64Gauge("system.memory.available",
			metric.WithDescription("Available memory in bytes"),
			metric.WithUnit("By"))
		if err != nil {
			return err
		}

		c.swapTotal, err = c.meter.Int64Gauge("system.swap.total",
			metric.WithDescription("Total swap in bytes"),
			metric.WithUnit("By"))
		if err != nil {
			return err
		}

		c.swapUsed, err = c.meter.Int64Gauge("system.swap.used",
			metric.WithDescription("Used swap in bytes"),
			metric.WithUnit("By"))
		if err != nil {
			return err
		}
	}

	// Disk metrics
	if c.cfg.Collectors.Disk || c.cfg.Collectors.Filesystem {
		c.diskTotal, err = c.meter.Int64Gauge("system.disk.total",
			metric.WithDescription("Total disk space in bytes"),
			metric.WithUnit("By"))
		if err != nil {
			return err
		}

		c.diskUsed, err = c.meter.Int64Gauge("system.disk.used",
			metric.WithDescription("Used disk space in bytes"),
			metric.WithUnit("By"))
		if err != nil {
			return err
		}

		c.diskFree, err = c.meter.Int64Gauge("system.disk.free",
			metric.WithDescription("Free disk space in bytes"),
			metric.WithUnit("By"))
		if err != nil {
			return err
		}

		c.diskUsedPercent, err = c.meter.Float64Gauge("system.disk.used_percent",
			metric.WithDescription("Disk usage percentage"),
			metric.WithUnit("%"))
		if err != nil {
			return err
		}

		c.diskReadBytes, err = c.meter.Int64Counter("system.disk.read_bytes",
			metric.WithDescription("Disk read bytes"),
			metric.WithUnit("By"))
		if err != nil {
			return err
		}

		c.diskWriteBytes, err = c.meter.Int64Counter("system.disk.write_bytes",
			metric.WithDescription("Disk write bytes"),
			metric.WithUnit("By"))
		if err != nil {
			return err
		}
	}

	// Network metrics
	if c.cfg.Collectors.Network {
		c.netBytesRecv, err = c.meter.Int64Counter("system.network.bytes_recv",
			metric.WithDescription("Network bytes received"),
			metric.WithUnit("By"))
		if err != nil {
			return err
		}

		c.netBytesSent, err = c.meter.Int64Counter("system.network.bytes_sent",
			metric.WithDescription("Network bytes sent"),
			metric.WithUnit("By"))
		if err != nil {
			return err
		}

		c.netPacketsRecv, err = c.meter.Int64Counter("system.network.packets_recv",
			metric.WithDescription("Network packets received"),
			metric.WithUnit("1"))
		if err != nil {
			return err
		}

		c.netPacketsSent, err = c.meter.Int64Counter("system.network.packets_sent",
			metric.WithDescription("Network packets sent"),
			metric.WithUnit("1"))
		if err != nil {
			return err
		}

		c.netErrsIn, err = c.meter.Int64Counter("system.network.errors_in",
			metric.WithDescription("Network input errors"),
			metric.WithUnit("1"))
		if err != nil {
			return err
		}

		c.netErrsOut, err = c.meter.Int64Counter("system.network.errors_out",
			metric.WithDescription("Network output errors"),
			metric.WithUnit("1"))
		if err != nil {
			return err
		}
	}

	// Load metrics
	if c.cfg.Collectors.Load {
		c.loadAvg1, err = c.meter.Float64Gauge("system.load.1m",
			metric.WithDescription("1 minute load average"),
			metric.WithUnit("1"))
		if err != nil {
			return err
		}

		c.loadAvg5, err = c.meter.Float64Gauge("system.load.5m",
			metric.WithDescription("5 minute load average"),
			metric.WithUnit("1"))
		if err != nil {
			return err
		}

		c.loadAvg15, err = c.meter.Float64Gauge("system.load.15m",
			metric.WithDescription("15 minute load average"),
			metric.WithUnit("1"))
		if err != nil {
			return err
		}
	}

	// Process metrics
	if c.cfg.Collectors.Process {
		c.processCount, err = c.meter.Int64Gauge("system.process.count",
			metric.WithDescription("Number of processes"),
			metric.WithUnit("1"))
		if err != nil {
			return err
		}
	}

	// Uptime
	c.uptime, err = c.meter.Int64Gauge("system.uptime",
		metric.WithDescription("System uptime in seconds"),
		metric.WithUnit("s"))
	if err != nil {
		return err
	}

	return nil
}

// Collect collects all enabled metrics.
func (c *MetricsCollector) Collect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var errs []error

	if c.cfg.Collectors.CPU {
		if err := c.collectCPU(ctx); err != nil {
			errs = append(errs, fmt.Errorf("cpu: %w", err))
		}
	}

	if c.cfg.Collectors.Memory {
		if err := c.collectMemory(ctx); err != nil {
			errs = append(errs, fmt.Errorf("memory: %w", err))
		}
	}

	if c.cfg.Collectors.Disk || c.cfg.Collectors.Filesystem {
		if err := c.collectDisk(ctx); err != nil {
			errs = append(errs, fmt.Errorf("disk: %w", err))
		}
	}

	if c.cfg.Collectors.Network {
		if err := c.collectNetwork(ctx); err != nil {
			errs = append(errs, fmt.Errorf("network: %w", err))
		}
	}

	if c.cfg.Collectors.Load {
		if err := c.collectLoad(ctx); err != nil {
			errs = append(errs, fmt.Errorf("load: %w", err))
		}
	}

	if c.cfg.Collectors.Process {
		if err := c.collectProcesses(ctx); err != nil {
			errs = append(errs, fmt.Errorf("process: %w", err))
		}
	}

	if err := c.collectUptime(ctx); err != nil {
		errs = append(errs, fmt.Errorf("uptime: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("collection errors: %v", errs)
	}

	return nil
}

func (c *MetricsCollector) collectCPU(ctx context.Context) error {
	cpuPercents, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		return err
	}

	if len(cpuPercents) > 0 {
		c.cpuUsage.Record(ctx, cpuPercents[0])
	}

	cpuTimes, err := cpu.TimesWithContext(ctx, false)
	if err != nil {
		return err
	}

	if len(cpuTimes) > 0 {
		total := cpuTimes[0].User + cpuTimes[0].System + cpuTimes[0].Idle +
			cpuTimes[0].Nice + cpuTimes[0].Iowait + cpuTimes[0].Irq +
			cpuTimes[0].Softirq + cpuTimes[0].Steal

		if total > 0 {
			c.cpuUser.Record(ctx, (cpuTimes[0].User/total)*100)
			c.cpuSystem.Record(ctx, (cpuTimes[0].System/total)*100)
			c.cpuIdle.Record(ctx, (cpuTimes[0].Idle/total)*100)
		}
	}

	return nil
}

func (c *MetricsCollector) collectMemory(ctx context.Context) error {
	vmem, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return err
	}

	c.memoryTotal.Record(ctx, int64(vmem.Total))
	c.memoryUsed.Record(ctx, int64(vmem.Used))
	c.memoryFree.Record(ctx, int64(vmem.Free))
	c.memoryAvailable.Record(ctx, int64(vmem.Available))

	swap, err := mem.SwapMemoryWithContext(ctx)
	if err != nil {
		return err
	}

	c.swapTotal.Record(ctx, int64(swap.Total))
	c.swapUsed.Record(ctx, int64(swap.Used))

	return nil
}

func (c *MetricsCollector) collectDisk(ctx context.Context) error {
	partitions, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return err
	}

	for _, partition := range partitions {
		usage, err := disk.UsageWithContext(ctx, partition.Mountpoint)
		if err != nil {
			c.logger.Debug("Failed to get disk usage",
				zap.String("mountpoint", partition.Mountpoint),
				zap.Error(err))
			continue
		}

		attrs := attribute.NewSet(
			attribute.String("device", partition.Device),
			attribute.String("mountpoint", partition.Mountpoint),
			attribute.String("fstype", partition.Fstype),
		)

		c.diskTotal.Record(ctx, int64(usage.Total), metric.WithAttributeSet(attrs))
		c.diskUsed.Record(ctx, int64(usage.Used), metric.WithAttributeSet(attrs))
		c.diskFree.Record(ctx, int64(usage.Free), metric.WithAttributeSet(attrs))
		c.diskUsedPercent.Record(ctx, usage.UsedPercent, metric.WithAttributeSet(attrs))
	}

	return nil
}

func (c *MetricsCollector) collectNetwork(ctx context.Context) error {
	netIO, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		return err
	}

	for _, io := range netIO {
		// Skip loopback
		if io.Name == "lo" {
			continue
		}

		attrs := attribute.NewSet(
			attribute.String("interface", io.Name),
		)

		c.netBytesRecv.Add(ctx, int64(io.BytesRecv), metric.WithAttributeSet(attrs))
		c.netBytesSent.Add(ctx, int64(io.BytesSent), metric.WithAttributeSet(attrs))
		c.netPacketsRecv.Add(ctx, int64(io.PacketsRecv), metric.WithAttributeSet(attrs))
		c.netPacketsSent.Add(ctx, int64(io.PacketsSent), metric.WithAttributeSet(attrs))
		c.netErrsIn.Add(ctx, int64(io.Errin), metric.WithAttributeSet(attrs))
		c.netErrsOut.Add(ctx, int64(io.Errout), metric.WithAttributeSet(attrs))
	}

	return nil
}

func (c *MetricsCollector) collectLoad(ctx context.Context) error {
	// Load average is only available on Unix systems
	if runtime.GOOS == "windows" {
		return nil
	}

	loadAvg, err := load.AvgWithContext(ctx)
	if err != nil {
		return err
	}

	c.loadAvg1.Record(ctx, loadAvg.Load1)
	c.loadAvg5.Record(ctx, loadAvg.Load5)
	c.loadAvg15.Record(ctx, loadAvg.Load15)

	return nil
}

func (c *MetricsCollector) collectProcesses(ctx context.Context) error {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return err
	}

	c.processCount.Record(ctx, int64(len(procs)))

	return nil
}

func (c *MetricsCollector) collectUptime(ctx context.Context) error {
	info, err := host.InfoWithContext(ctx)
	if err != nil {
		return err
	}

	c.uptime.Record(ctx, int64(info.Uptime))

	return nil
}

# Fuzz Tests for freader

This document describes the comprehensive fuzz test suite designed to validate the stability and robustness of freader's internal/collector under various file operation scenarios, including multi-file read/write operations, file rotation, and creation/deletion patterns.

## Overview

The fuzz tests are designed to validate collector robustness under various file operation scenarios:

- **Data loss prevention** - Verify no lines are lost during multi-file read/write operations
- **File rotation handling** - Ensure proper operation when files are rotated, moved, or renamed
- **Creation/deletion resilience** - Test behavior when files are rapidly created and deleted
- **Memory leaks** - Gradual memory accumulation under varying file patterns
- **Goroutine leaks** - Unclosed goroutines accumulating over time
- **Race conditions** - Timing-dependent bugs under file system stress
- **Performance degradation** - Slowdown after processing millions of lines
- **Resource exhaustion** - File descriptor leaks, connection issues
- **Chaos engineering** - Resilience under adverse file system conditions

## Test Types

### 1. Continuous Load Fuzz Test
**Duration**: 30 minutes - 8 hours
**Purpose**: Validate multi-file read/write operations without data loss

- 5 concurrent writers generating ~1000 lines/minute each
- Periodic file rotations every 5 minutes
- Real-time resource monitoring and adaptive behavior
- **Success Criteria**: >70% collection ratio, stable memory/goroutine usage, no data loss

### 2. Memory Stability Fuzz Test
**Duration**: 30 minutes - 2 hours
**Purpose**: Test memory behavior under varying file patterns and GC pressure

- Varying file sizes (1KB to 15MB with burst patterns)
- Complex file creation/cleanup patterns
- Detailed GC monitoring and memory pressure analysis
- **Success Criteria**: Bounded memory growth (<150MB), reasonable GC activity

### 3. Chaos Engineering Fuzz Test
**Duration**: 30 minutes - 1 hour
**Purpose**: Test file creation, deletion, and rotation handling under chaos

- 8 different chaos scenarios including rapid file creation/deletion
- Binary files, files without newlines, file corruption
- Large files (50KB-100KB) and burst creation/deletion storms
- File truncation, corruption simulation, and temporary files
- Error injection with configurable failure rates
- Adaptive polling based on system stress
- **Success Criteria**: Continues operating, bounded resource usage, <30% error rate

### 4. Goroutine Leak Detection Fuzz Test
**Duration**: 15-30 minutes
**Purpose**: Ensure proper resource cleanup during fuzz operations

- Multiple collector start/stop cycles under file system stress
- Monitor goroutine count over time with varying file patterns
- Validate graceful shutdown under load
- **Success Criteria**: ≤20 goroutine difference after cycles

## Running Fuzz Tests

### Local Development

```bash
# Run short version (30 seconds)
FREADER_FUZZ_TEST=1 FREADER_FUZZ_DURATION=30s \
  go test -v -timeout=5m -run="TestCollector_Fuzz" ./internal/collector

# Run medium fuzz test (30 minutes)
FREADER_FUZZ_TEST=1 FREADER_FUZZ_DURATION=30m \
  go test -v -timeout=1h -run="TestCollector_Fuzz" ./internal/collector

# Run specific fuzz test
FREADER_FUZZ_TEST=1 FREADER_FUZZ_DURATION=1h \
  go test -v -timeout=2h -run="TestCollector_Fuzz/ChaosEngineering" ./internal/collector
```

### CI/CD Integration

The fuzz tests run automatically in GitHub Actions:

- **Daily**: 1-hour fuzz tests at 2 AM UTC
- **Weekly**: 8-hour marathon fuzz test on Saturdays
- **Manual**: Trigger with custom duration

```yaml
# .github/workflows/fuzz-tests.yml
# Automatically runs comprehensive fuzz testing validation
```

## Monitoring & Alerts

### Key Metrics Tracked

1. **Collection Ratio**: Lines collected / Lines written
2. **Memory Usage**: GC frequency and heap growth
3. **Goroutine Count**: Leak detection over cycles
4. **Error Rate**: Failed operations / Total operations
5. **Latency**: Time from write to collection

### Alert Thresholds

- **Collection Ratio** < 70%: Performance degradation
- **GC Frequency** > 100 cycles/minute: Memory pressure
- **Goroutine Growth** > 50 per cycle: Resource leak
- **Error Rate** > 5%: Stability issues

### Failure Actions

1. **Create GitHub Issue**: Automatic issue creation with:
   - Run details and logs
   - Performance metrics
   - Suggested investigation steps

2. **Artifact Collection**:
   - Memory profiles
   - CPU profiles
   - Log files
   - Test data samples

## Expected Results

### Baseline Performance (30s fuzz test)

```
Duration: 30s
Lines written: ~1,500-2,000
Lines collected: ~1,400-1,900
Collection ratio: >90%
Chaos events: 5-10
Memory growth: <5MB
Goroutines: Stable (+5-10)
```

### Long-term Fuzz Stability (8h test)

```
Duration: 8h
Lines written: ~5-8 million
Lines collected: >3.5 million
Collection ratio: >70%
Chaos events: >1000
Memory: Bounded growth <200MB
Goroutines: Stable (±20)
Rotations: 96 successful rotations
Error rate: <30%
```

## Interpreting Results

### ✅ Healthy Results
- Collection ratio >80% consistently
- Stable memory usage over time
- Goroutine count remains bounded
- No error spikes or crashes

### ⚠️ Warning Signs
- Collection ratio dropping below 70%
- Memory usage growing linearly
- Goroutine count increasing per cycle
- Frequent GC activity (>50 cycles/min)

### ❌ Critical Issues
- Collection ratio <50%
- Memory leaks (>500MB growth)
- Goroutine leaks (>100 extra)
- Process crashes or hangs

## Troubleshooting

### Memory Issues
```bash
# Get memory profile during long test
go tool pprof http://localhost:6060/debug/pprof/heap

# Check for goroutine leaks
go tool pprof http://localhost:6060/debug/pprof/goroutine
```

### Performance Issues
```bash
# CPU profiling during test
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Trace analysis
go tool trace trace.out
```

### Data Consistency
- Check collector logs for errors
- Verify offset storage integrity
- Compare expected vs actual line counts

## Best Practices

### Before Running Long Tests
1. Ensure sufficient disk space (>10GB recommended)
2. Monitor system resources (CPU, memory, I/O)
3. Close other resource-intensive applications
4. Set appropriate timeout values

### During Test Execution
1. Monitor test progress logs
2. Watch for memory/CPU usage trends
3. Check for disk space consumption
4. Verify no interference from other processes

### After Test Completion
1. Review all collected metrics
2. Compare against baseline performance
3. Investigate any anomalies or warnings
4. Update performance baselines if needed

## Future Enhancements

- **Network FS fuzz testing**: Test file operations on networked storage
- **Container environment fuzzing**: Validate behavior in Docker/K8s under stress
- **Database integration fuzzing**: Offset storage testing under file chaos
- **Multi-platform fuzzing**: Extended tests on Windows/macOS
- **Benchmark regression with fuzz patterns**: Automated performance comparison
- **Advanced chaos scenarios**: Disk full, permission changes, symlink manipulation
- **Concurrent collector fuzzing**: Multiple collector instances on same files

---

**Note**: Fuzz tests are resource-intensive and simulate real-world file system stress. They should be run during off-hours or on dedicated test infrastructure to avoid impacting development workflows.
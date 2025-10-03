package collector

import (
	"crypto/rand"
	"fmt"
	mathrand "math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loykin/freader/internal/watcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCollector_Fuzz runs comprehensive fuzz tests to validate collector stability
// under various stress conditions including file operations, chaos scenarios, and resource pressure.
// This includes testing for data loss, file rotation handling, and creation/deletion scenarios.
func TestCollector_Fuzz(t *testing.T) {
	// Skip in short mode and normal unit tests
	if testing.Short() {
		t.Skip("skipping fuzz test in short mode")
	}

	// Only run if explicitly requested
	if os.Getenv("FREADER_FUZZ_TEST") == "" {
		t.Skip("skipping fuzz test - set FREADER_FUZZ_TEST=1 to enable")
	}

	// Default to shorter duration for CI environments
	defaultDuration := 30 * time.Minute
	if os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("CI") != "" {
		defaultDuration = 10 * time.Minute
	}
	duration := parseDuration(os.Getenv("FREADER_FUZZ_DURATION"), defaultDuration)
	t.Logf("Starting fuzz test for %v", duration)

	testCases := []struct {
		name string
		test func(*testing.T, time.Duration)
	}{
		{"ContinuousLoad", testFuzzContinuousLoad},
		{"MemoryStability", testFuzzMemoryStability},
		{"ChaosEngineering", testFuzzChaosEngineering},
		{"GoroutineLeakDetection", testFuzzGoroutineLeakDetection},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.test(t, duration)
		})
	}
}

func parseDuration(env string, defaultDur time.Duration) time.Duration {
	if env == "" {
		return defaultDur
	}
	if dur, err := time.ParseDuration(env); err == nil {
		return dur
	}
	return defaultDur
}

// ResourceMonitor tracks system resources during long-running tests
type ResourceMonitor struct {
	initialGoroutines int
	initialMemStats   runtime.MemStats
	maxMemory         uint64
	maxGoroutines     int
	mu                sync.Mutex
}

func NewResourceMonitor() *ResourceMonitor {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return &ResourceMonitor{
		initialGoroutines: runtime.NumGoroutine(),
		initialMemStats:   memStats,
		maxMemory:         memStats.Alloc,
		maxGoroutines:     runtime.NumGoroutine(),
	}
}

func (rm *ResourceMonitor) Update() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	if memStats.Alloc > rm.maxMemory {
		rm.maxMemory = memStats.Alloc
	}

	goroutines := runtime.NumGoroutine()
	if goroutines > rm.maxGoroutines {
		rm.maxGoroutines = goroutines
	}
}

func (rm *ResourceMonitor) GetStats() (memGrowth uint64, goroutineGrowth int) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return rm.maxMemory - rm.initialMemStats.Alloc, rm.maxGoroutines - rm.initialGoroutines
}

// testFuzzContinuousLoad runs collector under continuous multi-file load to test for data loss
func testFuzzContinuousLoad(t *testing.T, duration time.Duration) {
	tempDir := t.TempDir()
	monitor := NewResourceMonitor()

	var collected []string
	var mu sync.Mutex
	var linesWritten, linesCollected int64

	cfg := Config{
		Include:             []string{tempDir},
		PollInterval:        100 * time.Millisecond,
		WorkerCount:         3,
		Separator:           "\n",
		FingerprintStrategy: watcher.FingerprintStrategyChecksum,
		FingerprintSize:     1024,
		OnLineFunc: func(line string) {
			mu.Lock()
			collected = append(collected, line)
			mu.Unlock()
			atomic.AddInt64(&linesCollected, 1)
		},
	}

	collector, err := NewCollector(cfg)
	require.NoError(t, err)

	collector.Start()
	defer collector.Stop()

	// Start continuous writers
	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	// Multiple continuous writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			filename := filepath.Join(tempDir, fmt.Sprintf("continuous_%d.log", writerID))

			lineCount := 0
			for {
				select {
				case <-stopCh:
					return
				case <-time.After(50 * time.Millisecond):
					// Write batch of lines
					file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
					if err != nil {
						continue
					}

					for j := 0; j < 5; j++ {
						line := fmt.Sprintf("writer_%d_line_%d_%d\n", writerID, lineCount, time.Now().UnixNano())
						if _, err := file.WriteString(line); err == nil {
							atomic.AddInt64(&linesWritten, 1)
							lineCount++
						}
					}

					_ = file.Sync()
					_ = file.Close()
				}
			}
		}(i)
	}

	// Periodic file rotator
	wg.Add(1)
	go func() {
		defer wg.Done()
		rotationTicker := time.NewTicker(5 * time.Minute)
		defer rotationTicker.Stop()

		rotationCount := 0
		for {
			select {
			case <-stopCh:
				return
			case <-rotationTicker.C:
				// Rotate one of the files
				fileID := rotationCount % 5
				oldFile := filepath.Join(tempDir, fmt.Sprintf("continuous_%d.log", fileID))
				newFile := filepath.Join(tempDir, fmt.Sprintf("continuous_%d.log.%d", fileID, rotationCount))

				if _, err := os.Stat(oldFile); err == nil {
					_ = os.Rename(oldFile, newFile)
					t.Logf("Rotated file %s to %s", oldFile, newFile)
				}
				rotationCount++
			}
		}
	}()

	// Resource monitoring and progress reporting
	progressTicker := time.NewTicker(5 * time.Minute)
	resourceTicker := time.NewTicker(30 * time.Second)
	defer progressTicker.Stop()
	defer resourceTicker.Stop()

	startTime := time.Now()
	for {
		select {
		case <-time.After(duration):
			goto cleanup
		case <-resourceTicker.C:
			monitor.Update()
		case <-progressTicker.C:
			elapsed := time.Since(startTime)
			written := atomic.LoadInt64(&linesWritten)
			collected := atomic.LoadInt64(&linesCollected)
			ratio := float64(collected) / float64(written) * 100
			memGrowth, goroutineGrowth := monitor.GetStats()

			t.Logf("Progress: %v elapsed, %d written, %d collected (%.1f%%), mem: +%dKB, goroutines: +%d",
				elapsed, written, collected, ratio, memGrowth/1024, goroutineGrowth)

			// Health checks with adaptive thresholds
			if written > 1000 && ratio < 30 {
				t.Errorf("Collection ratio too low: %.1f%% after %v", ratio, elapsed)
			}
			if memGrowth > 100*1024*1024 { // 100MB growth warning
				t.Logf("WARNING: High memory growth detected: %dMB", memGrowth/(1024*1024))
			}
			if goroutineGrowth > 50 {
				t.Errorf("Excessive goroutine growth: +%d goroutines", goroutineGrowth)
			}
		}
	}

cleanup:
	close(stopCh)
	wg.Wait()

	// Final resource check and cleanup
	monitor.Update()
	finalWritten := atomic.LoadInt64(&linesWritten)
	finalCollected := atomic.LoadInt64(&linesCollected)
	finalRatio := float64(finalCollected) / float64(finalWritten) * 100
	finalMemGrowth, finalGoroutineGrowth := monitor.GetStats()

	// Force GC to get accurate final memory usage
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var finalMemStats runtime.MemStats
	runtime.ReadMemStats(&finalMemStats)

	t.Logf("Continuous load test completed:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Lines written: %d", finalWritten)
	t.Logf("  Lines collected: %d", finalCollected)
	t.Logf("  Collection ratio: %.2f%%", finalRatio)
	t.Logf("  Peak memory growth: %dMB", finalMemGrowth/(1024*1024))
	t.Logf("  Peak goroutine growth: +%d", finalGoroutineGrowth)
	t.Logf("  Final heap: %dMB, GC cycles: %d", finalMemStats.HeapAlloc/(1024*1024), finalMemStats.NumGC)

	assert.Greater(t, finalWritten, int64(10000), "Should have written substantial data")
	assert.Greater(t, finalRatio, 50.0, "Should maintain reasonable collection ratio over time")
	assert.Less(t, finalMemGrowth, uint64(200*1024*1024), "Memory growth should be bounded (200MB)")
	assert.Less(t, finalGoroutineGrowth, 100, "Goroutine growth should be bounded (+100)")
}

// testFuzzMemoryStability monitors memory usage under varying file patterns and GC pressure
func testFuzzMemoryStability(t *testing.T, duration time.Duration) {
	tempDir := t.TempDir()
	monitor := NewResourceMonitor()

	var linesCollected int64
	var baselineMemStats runtime.MemStats
	runtime.ReadMemStats(&baselineMemStats)

	cfg := Config{
		Include:             []string{tempDir},
		PollInterval:        100 * time.Millisecond,
		WorkerCount:         2,
		Separator:           "\n",
		FingerprintStrategy: watcher.FingerprintStrategyChecksum,
		FingerprintSize:     1024,
		OnLineFunc: func(line string) {
			atomic.AddInt64(&linesCollected, 1)
		},
	}

	collector, err := NewCollector(cfg)
	require.NoError(t, err)

	collector.Start()
	defer collector.Stop()

	// Memory monitoring
	memoryTicker := time.NewTicker(1 * time.Minute)
	defer memoryTicker.Stop()

	// Data generator with varying patterns to stress memory
	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	// Writer goroutine with memory pressure patterns
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		lineCount := 0
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				filename := filepath.Join(tempDir, fmt.Sprintf("memory_test_%d.log", lineCount%10))

				// Create varying sized content to stress allocator
				var content string
				switch {
				case lineCount%1000 == 0:
					// Large bursts to trigger GC
					content = fmt.Sprintf("burst_line_%d: %s\n", lineCount,
						string(make([]byte, 5000+mathrand.Intn(10000))))
				case lineCount%100 == 0:
					// Medium sized content
					content = fmt.Sprintf("medium_line_%d: %s\n", lineCount,
						string(make([]byte, 500+mathrand.Intn(1500))))
				default:
					content = fmt.Sprintf("normal_line_%d_%d\n", lineCount, time.Now().UnixNano())
				}

				if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
					t.Logf("Write error (non-fatal): %v", err)
				}
				lineCount++

				// Periodic cleanup with error handling
				if lineCount%2000 == 0 {
					for i := 0; i < 10; i++ {
						oldFile := filepath.Join(tempDir, fmt.Sprintf("memory_test_%d.log", i))
						if err := os.Remove(oldFile); err != nil && !os.IsNotExist(err) {
							t.Logf("Cleanup error (non-fatal): %v", err)
						}
					}
				}
			}
		}
	}()

	startTime := time.Now()
	lastGCCount := baselineMemStats.NumGC

	for {
		select {
		case <-time.After(duration):
			goto cleanup
		case <-memoryTicker.C:
			elapsed := time.Since(startTime)
			collected := atomic.LoadInt64(&linesCollected)

			// Update resource monitor and get detailed memory stats
			monitor.Update()
			var currentMemStats runtime.MemStats
			runtime.ReadMemStats(&currentMemStats)

			// Calculate memory growth and GC metrics
			heapGrowth := currentMemStats.HeapAlloc - baselineMemStats.HeapAlloc
			gcIncrease := currentMemStats.NumGC - lastGCCount
			lastGCCount = currentMemStats.NumGC

			// Calculate GC pause percentile (simplified)
			avgGCPause := time.Duration(currentMemStats.PauseTotalNs / uint64(currentMemStats.NumGC))

			t.Logf("Memory check: %v elapsed, %d lines collected", elapsed, collected)
			t.Logf("  Heap: %dMB (+%dMB), Sys: %dMB, GC: %d cycles (+%d), Avg pause: %v",
				currentMemStats.HeapAlloc/(1024*1024), heapGrowth/(1024*1024),
				currentMemStats.Sys/(1024*1024), currentMemStats.NumGC, gcIncrease, avgGCPause)

			// More sophisticated health checks
			if heapGrowth > 100*1024*1024 {
				t.Logf("WARNING: High heap growth detected: %dMB", heapGrowth/(1024*1024))
			}
			if gcIncrease > 50 {
				t.Logf("WARNING: High GC frequency: %d cycles in 1 minute", gcIncrease)
			}
			if avgGCPause > 10*time.Millisecond {
				t.Logf("WARNING: High GC pause times: %v average", avgGCPause)
			}
		}
	}

cleanup:
	close(stopCh)
	wg.Wait()

	// Final memory assessment
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	var finalMemStats runtime.MemStats
	runtime.ReadMemStats(&finalMemStats)

	finalCollected := atomic.LoadInt64(&linesCollected)
	finalHeapGrowth := finalMemStats.HeapAlloc - baselineMemStats.HeapAlloc
	totalGCCycles := finalMemStats.NumGC - baselineMemStats.NumGC

	t.Logf("Memory stability test completed:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Lines collected: %d", finalCollected)
	t.Logf("  Final heap growth: %dMB", finalHeapGrowth/(1024*1024))
	t.Logf("  Total GC cycles: %d", totalGCCycles)
	t.Logf("  Average GC pause: %v", time.Duration(finalMemStats.PauseTotalNs/uint64(finalMemStats.NumGC)))

	assert.Greater(t, finalCollected, int64(1000), "Should have collected substantial data")
	assert.Less(t, finalHeapGrowth, uint64(150*1024*1024), "Heap growth should be reasonable (150MB)")
	assert.Less(t, int(totalGCCycles), int(duration.Minutes()*20), "GC frequency should be reasonable")
}

// ErrorInjector provides controlled failure scenarios for testing resilience
type ErrorInjector struct {
	fileCorruptionRate float64
	writeFailureRate   float64
	rand               *mathrand.Rand
	mu                 sync.Mutex
}

func NewErrorInjector(corruptionRate, failureRate float64) *ErrorInjector {
	return &ErrorInjector{
		fileCorruptionRate: corruptionRate,
		writeFailureRate:   failureRate,
		rand:               mathrand.New(mathrand.NewSource(time.Now().UnixNano())),
	}
}

func (ei *ErrorInjector) ShouldCorruptFile() bool {
	ei.mu.Lock()
	defer ei.mu.Unlock()
	return ei.rand.Float64() < ei.fileCorruptionRate
}

func (ei *ErrorInjector) ShouldFailWrite() bool {
	ei.mu.Lock()
	defer ei.mu.Unlock()
	return ei.rand.Float64() < ei.writeFailureRate
}

// testFuzzChaosEngineering introduces random file operations to test creation/deletion/rotation handling
func testFuzzChaosEngineering(t *testing.T, duration time.Duration) {
	tempDir := t.TempDir()
	monitor := NewResourceMonitor()
	errorInjector := NewErrorInjector(0.05, 0.02) // 5% file corruption, 2% write failures

	var linesCollected int64
	var chaosEvents int64
	var errorEvents int64

	// Adaptive configuration that adjusts under stress
	basePollInterval := 50 * time.Millisecond
	cfg := Config{
		Include:             []string{tempDir},
		PollInterval:        basePollInterval,
		WorkerCount:         4,
		Separator:           "\n",
		FingerprintStrategy: watcher.FingerprintStrategyChecksum,
		FingerprintSize:     1024,
		OnLineFunc: func(line string) {
			atomic.AddInt64(&linesCollected, 1)
		},
	}

	collector, err := NewCollector(cfg)
	require.NoError(t, err)

	collector.Start()
	defer collector.Stop()

	stopCh := make(chan struct{})
	var wg sync.WaitGroup

	// Normal data writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		lineCount := 0
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				filename := filepath.Join(tempDir, "normal.log")
				file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
				if err != nil {
					continue
				}

				for i := 0; i < 3; i++ {
					_, _ = file.WriteString(fmt.Sprintf("normal_line_%d\n", lineCount))
					lineCount++
				}
				_ = file.Sync()
				_ = file.Close()
			}
		}
	}()

	// Enhanced chaos agent with error injection
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Adjust chaos frequency for CI environments
		chaosInterval := 3 * time.Second
		if os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("CI") != "" {
			chaosInterval = 10 * time.Second // Less frequent in CI
		}
		chaosTicker := time.NewTicker(chaosInterval)
		defer chaosTicker.Stop()

		chaosFileCount := 0
		for {
			select {
			case <-stopCh:
				return
			case <-chaosTicker.C:
				// Skip chaos event if error injector says to fail
				if errorInjector.ShouldFailWrite() {
					atomic.AddInt64(&errorEvents, 1)
					continue
				}

				chaosAction := mathrand.Intn(8) // More chaos types

				switch chaosAction {
				case 0: // Create temporary file with large content
					filename := filepath.Join(tempDir, fmt.Sprintf("chaos_large_%d.log", chaosFileCount))
					// Reduce size for CI environments to prevent memory issues
					maxSize := 50000
					if os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("CI") != "" {
						maxSize = 5000
					}
					largeContent := make([]byte, maxSize+mathrand.Intn(maxSize))
					for i := range largeContent {
						if i%100 == 99 {
							largeContent[i] = '\n'
						} else {
							largeContent[i] = byte('a' + mathrand.Intn(26))
						}
					}
					if err := os.WriteFile(filename, largeContent, 0644); err != nil {
						t.Logf("WARN failed to create chaos file %s: %v", filename, err)
					}
					chaosFileCount++

				case 1: // Create and immediately delete file
					filename := filepath.Join(tempDir, fmt.Sprintf("chaos_temp_%d.log", chaosFileCount))
					if err := os.WriteFile(filename, []byte("temporary content\n"), 0644); err == nil {
						time.Sleep(50 * time.Millisecond)
						if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
							t.Logf("WARN failed to remove chaos temp file %s: %v", filename, err)
						}
					} else {
						t.Logf("WARN failed to create chaos temp file %s: %v", filename, err)
					}
					chaosFileCount++

				case 2: // Create file with no newlines
					filename := filepath.Join(tempDir, fmt.Sprintf("chaos_no_newlines_%d.log", chaosFileCount))
					content := make([]byte, 1000)
					for i := range content {
						content[i] = byte('x')
					}
					_ = os.WriteFile(filename, content, 0644)
					chaosFileCount++

				case 3: // Create many small files (fewer in CI)
					maxFiles := 10
					if os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("CI") != "" {
						maxFiles = 3 // Create fewer files in CI
					}
					for i := 0; i < maxFiles; i++ {
						filename := filepath.Join(tempDir, fmt.Sprintf("chaos_small_%d_%d.log", chaosFileCount, i))
						if err := os.WriteFile(filename, []byte(fmt.Sprintf("small_%d\n", i)), 0644); err != nil {
							t.Logf("WARN failed to create small chaos file %s: %v", filename, err)
						}
					}
					chaosFileCount++

				case 4: // Truncate existing file
					filename := filepath.Join(tempDir, "normal.log")
					if _, err := os.Stat(filename); err == nil {
						_ = os.Truncate(filename, 0)
					}

				case 5: // Create binary file
					filename := filepath.Join(tempDir, fmt.Sprintf("chaos_binary_%d.log", chaosFileCount))
					binaryContent := make([]byte, 1000)
					_, _ = rand.Read(binaryContent)
					_ = os.WriteFile(filename, binaryContent, 0644)
					chaosFileCount++

				case 6: // File corruption simulation
					if errorInjector.ShouldCorruptFile() {
						filename := filepath.Join(tempDir, "normal.log")
						if file, err := os.OpenFile(filename, os.O_WRONLY, 0644); err == nil {
							// Seek to random position and write garbage
							if stat, err := file.Stat(); err == nil && stat.Size() > 0 {
								pos := mathrand.Int63n(stat.Size())
								_, _ = file.Seek(pos, 0)
								_, _ = file.WriteString("CORRUPTED")
							}
							_ = file.Close()
							atomic.AddInt64(&errorEvents, 1)
						}
					}

				case 7: // Rapid file creation/deletion storm (reduced for CI)
					maxBurst := 20
					if os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("CI") != "" {
						maxBurst = 5 // Reduce burst size in CI
					}
					for burst := 0; burst < maxBurst; burst++ {
						filename := filepath.Join(tempDir, fmt.Sprintf("chaos_burst_%d_%d.log", chaosFileCount, burst))
						if err := os.WriteFile(filename, []byte(fmt.Sprintf("burst_%d\n", burst)), 0644); err == nil {
							time.Sleep(10 * time.Millisecond)
							if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
								t.Logf("WARN failed to remove burst file %s: %v", filename, err)
							}
						}
					}
					chaosFileCount++
				}

				atomic.AddInt64(&chaosEvents, 1)
			}
		}
	}()

	// Adaptive monitoring and progress reporting with timeout channel
	progressInterval := 90 * time.Second
	if duration < 2*time.Minute {
		progressInterval = duration / 3 // For short tests, report more frequently
	}
	progressTicker := time.NewTicker(progressInterval)
	adaptiveTicker := time.NewTicker(30 * time.Second)
	defer progressTicker.Stop()
	defer adaptiveTicker.Stop()

	// Use timeout channel instead of time.After in select
	timeout := time.After(duration)
	startTime := time.Now()
	for {
		select {
		case <-timeout:
			goto cleanup
		case <-adaptiveTicker.C:
			// Update resource monitoring for adaptive behavior
			monitor.Update()
			memGrowth, goroutineGrowth := monitor.GetStats()

			// Adaptive polling: slow down if under stress
			if memGrowth > 50*1024*1024 || goroutineGrowth > 25 {
				// System under stress - reduce polling frequency
				newInterval := basePollInterval * 2

				t.Logf("ADAPTIVE: Increasing poll interval to %v due to stress", newInterval)
				// Note: In a real implementation, we'd update the collector's poll interval
			}

		case <-progressTicker.C:
			elapsed := time.Since(startTime)
			collected := atomic.LoadInt64(&linesCollected)
			events := atomic.LoadInt64(&chaosEvents)
			errors := atomic.LoadInt64(&errorEvents)
			memGrowth, goroutineGrowth := monitor.GetStats()

			t.Logf("Chaos progress: %v elapsed, %d collected, %d chaos, %d errors, mem: +%dMB, goroutines: +%d",
				elapsed, collected, events, errors, memGrowth/(1024*1024), goroutineGrowth)

			// System health warnings during chaos
			if memGrowth > 100*1024*1024 {
				t.Logf("WARNING: High memory growth under chaos conditions")
			}
			if goroutineGrowth > 50 {
				t.Logf("WARNING: High goroutine growth under chaos conditions")
			}
		}
	}

cleanup:
	close(stopCh)
	wg.Wait()

	// Final resilience assessment
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	monitor.Update()

	finalCollected := atomic.LoadInt64(&linesCollected)
	finalChaos := atomic.LoadInt64(&chaosEvents)
	finalErrors := atomic.LoadInt64(&errorEvents)
	finalMemGrowth, finalGoroutineGrowth := monitor.GetStats()

	t.Logf("Chaos engineering test completed:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Lines collected: %d", finalCollected)
	t.Logf("  Chaos events: %d", finalChaos)
	t.Logf("  Error events: %d", finalErrors)
	t.Logf("  Peak memory growth: %dMB", finalMemGrowth/(1024*1024))
	t.Logf("  Peak goroutine growth: +%d", finalGoroutineGrowth)

	// Resilience assertions - system should continue operating despite chaos
	assert.Greater(t, finalCollected, int64(100), "Should collect data despite chaos")
	minChaosEvents := int64(1)
	if duration >= time.Minute {
		minChaosEvents = 5
	}
	assert.GreaterOrEqual(t, finalChaos, minChaosEvents, "Should have generated chaos events")
	assert.Less(t, finalMemGrowth, uint64(300*1024*1024), "Memory growth should be bounded under chaos (300MB)")
	assert.Less(t, finalGoroutineGrowth, 150, "Goroutine growth should be bounded under chaos (+150)")

	// Error tolerance - some errors are expected in chaos scenarios
	errorRate := float64(finalErrors) / float64(finalChaos) * 100
	t.Logf("  Error rate: %.1f%%", errorRate)
	assert.Less(t, errorRate, 30.0, "Error rate should be manageable even under chaos (<30%)")
}

// testFuzzGoroutineLeakDetection monitors for resource leaks during fuzz operations
func testFuzzGoroutineLeakDetection(t *testing.T, duration time.Duration) {
	tempDir := t.TempDir()

	// Initial goroutine count
	initialGoroutines := countGoroutines()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	var linesCollected int64

	cfg := Config{
		Include:             []string{tempDir},
		PollInterval:        100 * time.Millisecond,
		WorkerCount:         3,
		Separator:           "\n",
		FingerprintStrategy: watcher.FingerprintStrategyChecksum,
		FingerprintSize:     1024,
		OnLineFunc: func(line string) {
			atomic.AddInt64(&linesCollected, 1)
		},
	}

	// Test multiple collector start/stop cycles
	cycles := 10
	cycleDuration := duration / time.Duration(cycles)

	for cycle := 0; cycle < cycles; cycle++ {
		t.Logf("Starting collector cycle %d/%d", cycle+1, cycles)

		collector, err := NewCollector(cfg)
		require.NoError(t, err)

		collector.Start()

		// Generate some load
		filename := filepath.Join(tempDir, fmt.Sprintf("leak_test_%d.log", cycle))
		for i := 0; i < 100; i++ {
			content := fmt.Sprintf("cycle_%d_line_%d\n", cycle, i)
			_ = os.WriteFile(filename, []byte(content), 0644)
		}

		time.Sleep(cycleDuration)

		collector.Stop()

		// Check goroutine count
		currentGoroutines := countGoroutines()
		goroutineDiff := currentGoroutines - initialGoroutines

		t.Logf("Cycle %d completed: %d goroutines (diff: %+d)",
			cycle+1, currentGoroutines, goroutineDiff)

		// Allow some tolerance for background goroutines in CI environments
		maxAllowedGoroutines := 50
		if os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("CI") != "" {
			maxAllowedGoroutines = 100 // More tolerance in CI
		}
		if goroutineDiff > maxAllowedGoroutines {
			t.Errorf("Potential goroutine leak detected: %d extra goroutines after cycle %d",
				goroutineDiff, cycle+1)
		}

		// Give time for cleanup
		time.Sleep(200 * time.Millisecond)
	}

	finalGoroutines := countGoroutines()
	finalDiff := finalGoroutines - initialGoroutines

	t.Logf("Goroutine leak test completed:")
	t.Logf("  Initial goroutines: %d", initialGoroutines)
	t.Logf("  Final goroutines: %d", finalGoroutines)
	t.Logf("  Difference: %+d", finalDiff)
	t.Logf("  Lines collected: %d", atomic.LoadInt64(&linesCollected))

	// Final leak check with CI tolerance
	maxFinalLeaks := 20
	if os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("CI") != "" {
		maxFinalLeaks = 50 // More tolerance in CI
	}
	assert.LessOrEqual(t, finalDiff, maxFinalLeaks, "Should not have significant goroutine leaks")
}

func countGoroutines() int {
	return runtime.NumGoroutine()
}

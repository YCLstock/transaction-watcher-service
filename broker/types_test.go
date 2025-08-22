package broker

import (
	"sync"
	"testing"
	"time"
)

func TestNewMessage(t *testing.T) {
	id := "test-message-1"
	body := []byte("test message body")
	queue := "test-queue"
	
	msg := NewMessage(id, body, queue)
	
	if msg.ID != id {
		t.Errorf("Expected message ID %s, got %s", id, msg.ID)
	}
	
	if string(msg.Body) != string(body) {
		t.Errorf("Expected message body %s, got %s", string(body), string(msg.Body))
	}
	
	if msg.Queue != queue {
		t.Errorf("Expected queue %s, got %s", queue, msg.Queue)
	}
	
	if msg.Attempts != 0 {
		t.Errorf("Expected attempts 0, got %d", msg.Attempts)
	}
	
	if msg.MaxRetry != 3 {
		t.Errorf("Expected max retry 3, got %d", msg.MaxRetry)
	}
	
	if msg.Headers == nil {
		t.Error("Expected headers to be initialized")
	}
	
	if msg.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}
}

func TestNewMetrics(t *testing.T) {
	metrics := NewMetrics()
	
	if metrics.StartTime.IsZero() {
		t.Error("Expected start time to be set")
	}
	
	if metrics.QueueMetrics == nil {
		t.Error("Expected queue metrics to be initialized")
	}
	
	if len(metrics.QueueMetrics) != 0 {
		t.Error("Expected queue metrics to be empty initially")
	}
}

func TestMetricsAtomicOperations(t *testing.T) {
	metrics := NewMetrics()
	
	// 測試原子操作
	metrics.IncrementTotalMessages()
	metrics.IncrementProcessedMessages()
	metrics.IncrementFailedMessages()
	
	stats := metrics.GetStats()
	
	if stats["total_messages"].(int64) != 1 {
		t.Errorf("Expected total_messages 1, got %d", stats["total_messages"])
	}
	
	if stats["processed_messages"].(int64) != 1 {
		t.Errorf("Expected processed_messages 1, got %d", stats["processed_messages"])
	}
	
	if stats["failed_messages"].(int64) != 1 {
		t.Errorf("Expected failed_messages 1, got %d", stats["failed_messages"])
	}
}

func TestMetricsConcurrentAccess(t *testing.T) {
	metrics := NewMetrics()
	const numGoroutines = 100
	const incrementsPerGoroutine = 100
	
	var wg sync.WaitGroup
	
	// 啟動多個 goroutine 同時增加計數器
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				metrics.IncrementTotalMessages()
				metrics.IncrementProcessedMessages()
				metrics.IncrementFailedMessages()
			}
		}()
	}
	
	wg.Wait()
	
	expectedCount := int64(numGoroutines * incrementsPerGoroutine)
	stats := metrics.GetStats()
	
	if stats["total_messages"].(int64) != expectedCount {
		t.Errorf("Expected total_messages %d, got %d", expectedCount, stats["total_messages"])
	}
	
	if stats["processed_messages"].(int64) != expectedCount {
		t.Errorf("Expected processed_messages %d, got %d", expectedCount, stats["processed_messages"])
	}
	
	if stats["failed_messages"].(int64) != expectedCount {
		t.Errorf("Expected failed_messages %d, got %d", expectedCount, stats["failed_messages"])
	}
}

func TestQueueStatsCreation(t *testing.T) {
	stats := &QueueStats{
		Name:            "test-queue",
		MessageCount:    10,
		ConsumerCount:   2,
		EnqueuedTotal:   100,
		DequeuedTotal:   90,
		DeadLetterCount: 5,
	}
	
	if stats.Name != "test-queue" {
		t.Errorf("Expected name test-queue, got %s", stats.Name)
	}
	
	if stats.MessageCount != 10 {
		t.Errorf("Expected message count 10, got %d", stats.MessageCount)
	}
	
	if stats.ConsumerCount != 2 {
		t.Errorf("Expected consumer count 2, got %d", stats.ConsumerCount)
	}
}

func TestMetricsUptime(t *testing.T) {
	metrics := NewMetrics()
	
	// 等待一小段時間
	time.Sleep(10 * time.Millisecond)
	
	stats := metrics.GetStats()
	uptime := stats["uptime_seconds"].(float64)
	
	if uptime <= 0 {
		t.Errorf("Expected positive uptime, got %f", uptime)
	}
	
	if uptime > 1 { // 不應該超過1秒
		t.Errorf("Uptime seems too high: %f", uptime)
	}
}

func TestMetricsCopyQueueMetrics(t *testing.T) {
	metrics := NewMetrics()
	
	// 添加一些隊列指標
	metrics.QueueMetrics["queue1"] = &QueueStats{
		Name:         "queue1",
		MessageCount: 5,
	}
	
	stats := metrics.GetStats()
	queueMetrics := stats["queue_metrics"].(map[string]*QueueStats)
	
	if len(queueMetrics) != 1 {
		t.Errorf("Expected 1 queue metric, got %d", len(queueMetrics))
	}
	
	if queueMetrics["queue1"].Name != "queue1" {
		t.Errorf("Expected queue1, got %s", queueMetrics["queue1"].Name)
	}
	
	if queueMetrics["queue1"].MessageCount != 5 {
		t.Errorf("Expected message count 5, got %d", queueMetrics["queue1"].MessageCount)
	}
}
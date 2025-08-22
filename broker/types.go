package broker

import (
	"sync"
	"sync/atomic"
	"time"
)

// Message 表示訊息佇列中的基本消息單元
type Message struct {
	ID        string            `json:"id"`
	Body      []byte            `json:"body"`
	Headers   map[string]string `json:"headers,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Attempts  int               `json:"attempts"`
	MaxRetry  int               `json:"max_retry"`
	Queue     string            `json:"queue"`
}

// Queue 表示一個消息隊列的統計信息
type QueueStats struct {
	Name           string `json:"name"`
	MessageCount   int64  `json:"message_count"`
	ConsumerCount  int32  `json:"consumer_count"`
	EnqueuedTotal  int64  `json:"enqueued_total"`
	DequeuedTotal  int64  `json:"dequeued_total"`
	DeadLetterCount int64  `json:"dead_letter_count"`
}

// Metrics 包含 Broker 的運行指標
type Metrics struct {
	// 使用 atomic 操作保證線程安全
	TotalMessages     int64 // 總消息數
	ProcessedMessages int64 // 已處理消息數
	FailedMessages    int64 // 失敗消息數
	ActiveQueues      int32 // 活躍隊列數
	ActiveConsumers   int32 // 活躍消費者數
	StartTime         time.Time
	mu                sync.RWMutex
	QueueMetrics      map[string]*QueueStats
}

// IncrementTotalMessages 原子性地增加總消息數
func (m *Metrics) IncrementTotalMessages() {
	atomic.AddInt64(&m.TotalMessages, 1)
}

// IncrementProcessedMessages 原子性地增加已處理消息數
func (m *Metrics) IncrementProcessedMessages() {
	atomic.AddInt64(&m.ProcessedMessages, 1)
}

// IncrementFailedMessages 原子性地增加失敗消息數
func (m *Metrics) IncrementFailedMessages() {
	atomic.AddInt64(&m.FailedMessages, 1)
}

// GetStats 返回當前統計信息的快照
func (m *Metrics) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return map[string]interface{}{
		"total_messages":     atomic.LoadInt64(&m.TotalMessages),
		"processed_messages": atomic.LoadInt64(&m.ProcessedMessages),
		"failed_messages":    atomic.LoadInt64(&m.FailedMessages),
		"active_queues":      atomic.LoadInt32(&m.ActiveQueues),
		"active_consumers":   atomic.LoadInt32(&m.ActiveConsumers),
		"uptime_seconds":     time.Since(m.StartTime).Seconds(),
		"queue_metrics":      m.copyQueueMetrics(),
	}
}

// copyQueueMetrics 創建隊列指標的副本
func (m *Metrics) copyQueueMetrics() map[string]*QueueStats {
	result := make(map[string]*QueueStats)
	for name, stats := range m.QueueMetrics {
		result[name] = &QueueStats{
			Name:            stats.Name,
			MessageCount:    atomic.LoadInt64(&stats.MessageCount),
			ConsumerCount:   atomic.LoadInt32(&stats.ConsumerCount),
			EnqueuedTotal:   atomic.LoadInt64(&stats.EnqueuedTotal),
			DequeuedTotal:   atomic.LoadInt64(&stats.DequeuedTotal),
			DeadLetterCount: atomic.LoadInt64(&stats.DeadLetterCount),
		}
	}
	return result
}

// Broker 定義消息代理的核心接口
type Broker interface {
	// Queue 模式 (點對點)
	Push(queue string, msg Message) error
	Pull(queue string) (*Message, error)
	PullWithTimeout(queue string, timeout time.Duration) (*Message, error)
	
	// Pub/Sub 模式 (廣播)
	Publish(topic string, msg Message) error
	Subscribe(topic string) (<-chan Message, error)
	Unsubscribe(topic string, subscriber <-chan Message) error
	
	// Dead Letter Queue 處理
	GetDLQ(queue string) []Message
	MoveToDLQ(queue string, msg Message) error
	ReprocessDLQ(queue string, msgID string) error
	
	// 管理和監控
	GetQueueStats(queue string) (*QueueStats, error)
	GetMetrics() *Metrics
	GetAllQueues() []string
	PurgeQueue(queue string) error
	
	// 生命周期管理
	Close() error
	IsHealthy() bool
}

// SubscriberInfo 存儲訂閱者信息
type SubscriberInfo struct {
	Channel   chan Message
	Topic     string
	CreatedAt time.Time
}

// NewMetrics 創建新的指標實例
func NewMetrics() *Metrics {
	return &Metrics{
		StartTime:    time.Now(),
		QueueMetrics: make(map[string]*QueueStats),
	}
}

// NewMessage 創建新的消息實例
func NewMessage(id string, body []byte, queue string) Message {
	return Message{
		ID:        id,
		Body:      body,
		Headers:   make(map[string]string),
		Timestamp: time.Now(),
		Attempts:  0,
		MaxRetry:  3, // 默認重試3次
		Queue:     queue,
	}
}
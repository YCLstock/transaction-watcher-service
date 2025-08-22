package broker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// SimpleBroker 是一個高性能的內存消息代理實現
type SimpleBroker struct {
	// 使用 sync.Map 來實現無鎖的並發安全 map
	queues      sync.Map // map[string]*messageQueue
	subscribers sync.Map // map[string]*subscriberManager
	deadLetters sync.Map // map[string][]Message
	
	metrics *Metrics
	closed  int32
	ctx     context.Context
	cancel  context.CancelFunc
}

// messageQueue 表示一個消息隊列的實現
type messageQueue struct {
	name     string
	messages chan Message
	stats    *QueueStats
	mu       sync.RWMutex
}

// subscriberManager 管理一個主題的所有訂閱者
type subscriberManager struct {
	topic       string
	subscribers []chan Message
	mu          sync.RWMutex
}

// NewSimpleBroker 創建一個新的 SimpleBroker 實例
func NewSimpleBroker() *SimpleBroker {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &SimpleBroker{
		metrics: NewMetrics(),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Push 將消息推送到指定隊列 (Queue 模式 - 點對點)
func (b *SimpleBroker) Push(queue string, msg Message) error {
	if atomic.LoadInt32(&b.closed) == 1 {
		return fmt.Errorf("broker is closed")
	}
	
	msg.Queue = queue
	msg.Timestamp = time.Now()
	
	// 獲取或創建隊列
	queueInterface, _ := b.queues.LoadOrStore(queue, b.createMessageQueue(queue))
	mq := queueInterface.(*messageQueue)
	
	// 使用 select 實現非阻塞發送，避免死鎖
	select {
	case mq.messages <- msg:
		// 成功發送，更新統計
		atomic.AddInt64(&mq.stats.MessageCount, 1)
		atomic.AddInt64(&mq.stats.EnqueuedTotal, 1)
		b.metrics.IncrementTotalMessages()
		return nil
	default:
		// 隊列已滿，移動到死信隊列
		return b.MoveToDLQ(queue, msg)
	}
}

// Pull 從指定隊列拉取消息 (Queue 模式 - 點對點)
func (b *SimpleBroker) Pull(queue string) (*Message, error) {
	return b.PullWithTimeout(queue, 0)
}

// PullWithTimeout 從指定隊列拉取消息，支持超時
func (b *SimpleBroker) PullWithTimeout(queue string, timeout time.Duration) (*Message, error) {
	if atomic.LoadInt32(&b.closed) == 1 {
		return nil, fmt.Errorf("broker is closed")
	}
	
	queueInterface, exists := b.queues.Load(queue)
	if !exists {
		return nil, fmt.Errorf("queue %s does not exist", queue)
	}
	
	mq := queueInterface.(*messageQueue)
	
	if timeout == 0 {
		// 非阻塞模式
		select {
		case msg := <-mq.messages:
			atomic.AddInt64(&mq.stats.MessageCount, -1)
			atomic.AddInt64(&mq.stats.DequeuedTotal, 1)
			b.metrics.IncrementProcessedMessages()
			return &msg, nil
		default:
			return nil, nil // 沒有消息
		}
	}
	
	// 阻塞模式，支持超時
	ctx, cancel := context.WithTimeout(b.ctx, timeout)
	defer cancel()
	
	select {
	case msg := <-mq.messages:
		atomic.AddInt64(&mq.stats.MessageCount, -1)
		atomic.AddInt64(&mq.stats.DequeuedTotal, 1)
		b.metrics.IncrementProcessedMessages()
		return &msg, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("timeout waiting for message from queue %s", queue)
	}
}

// Publish 發布消息到指定主題 (Pub/Sub 模式 - 廣播)
func (b *SimpleBroker) Publish(topic string, msg Message) error {
	if atomic.LoadInt32(&b.closed) == 1 {
		return fmt.Errorf("broker is closed")
	}
	
	msg.Timestamp = time.Now()
	b.metrics.IncrementTotalMessages()
	
	subMgrInterface, exists := b.subscribers.Load(topic)
	if !exists {
		// 沒有訂閱者，直接返回
		return nil
	}
	
	subMgr := subMgrInterface.(*subscriberManager)
	subMgr.mu.RLock()
	defer subMgr.mu.RUnlock()
	
	// 向所有訂閱者廣播消息
	for _, subscriber := range subMgr.subscribers {
		select {
		case subscriber <- msg:
			// 成功發送
		default:
			// 訂閱者的緩衝區已滿，跳過
		}
	}
	
	return nil
}

// Subscribe 訂閱指定主題
func (b *SimpleBroker) Subscribe(topic string) (<-chan Message, error) {
	if atomic.LoadInt32(&b.closed) == 1 {
		return nil, fmt.Errorf("broker is closed")
	}
	
	// 創建一個有緩衝的通道給訂閱者
	subscriberChan := make(chan Message, 100)
	
	// 獲取或創建訂閱管理器
	subMgrInterface, _ := b.subscribers.LoadOrStore(topic, &subscriberManager{
		topic:       topic,
		subscribers: make([]chan Message, 0),
	})
	
	subMgr := subMgrInterface.(*subscriberManager)
	subMgr.mu.Lock()
	subMgr.subscribers = append(subMgr.subscribers, subscriberChan)
	subMgr.mu.Unlock()
	
	atomic.AddInt32(&b.metrics.ActiveConsumers, 1)
	
	return subscriberChan, nil
}

// Unsubscribe 取消訂閱
func (b *SimpleBroker) Unsubscribe(topic string, subscriber <-chan Message) error {
	subMgrInterface, exists := b.subscribers.Load(topic)
	if !exists {
		return fmt.Errorf("topic %s does not exist", topic)
	}
	
	subMgr := subMgrInterface.(*subscriberManager)
	subMgr.mu.Lock()
	defer subMgr.mu.Unlock()
	
	// 找到並移除訂閱者
	for i, sub := range subMgr.subscribers {
		if sub == subscriber {
			subMgr.subscribers = append(subMgr.subscribers[:i], subMgr.subscribers[i+1:]...)
			close(sub)
			atomic.AddInt32(&b.metrics.ActiveConsumers, -1)
			break
		}
	}
	
	return nil
}

// GetDLQ 獲取指定隊列的死信消息
func (b *SimpleBroker) GetDLQ(queue string) []Message {
	dlqInterface, exists := b.deadLetters.Load(queue)
	if !exists {
		return []Message{}
	}
	
	return dlqInterface.([]Message)
}

// MoveToDLQ 將消息移動到死信隊列
func (b *SimpleBroker) MoveToDLQ(queue string, msg Message) error {
	msg.Attempts++
	
	dlqInterface, _ := b.deadLetters.LoadOrStore(queue, []Message{})
	dlq := dlqInterface.([]Message)
	dlq = append(dlq, msg)
	b.deadLetters.Store(queue, dlq)
	
	// 更新統計
	queueInterface, exists := b.queues.Load(queue)
	if exists {
		mq := queueInterface.(*messageQueue)
		atomic.AddInt64(&mq.stats.DeadLetterCount, 1)
	}
	
	b.metrics.IncrementFailedMessages()
	return nil
}

// ReprocessDLQ 重新處理死信隊列中的消息
func (b *SimpleBroker) ReprocessDLQ(queue string, msgID string) error {
	dlqInterface, exists := b.deadLetters.Load(queue)
	if !exists {
		return fmt.Errorf("no dead letters for queue %s", queue)
	}
	
	dlq := dlqInterface.([]Message)
	for i, msg := range dlq {
		if msg.ID == msgID {
			// 重置嘗試次數
			msg.Attempts = 0
			
			// 從死信隊列中移除
			dlq = append(dlq[:i], dlq[i+1:]...)
			b.deadLetters.Store(queue, dlq)
			
			// 重新推送到隊列
			return b.Push(queue, msg)
		}
	}
	
	return fmt.Errorf("message %s not found in dead letter queue", msgID)
}

// GetQueueStats 獲取指定隊列的統計信息
func (b *SimpleBroker) GetQueueStats(queue string) (*QueueStats, error) {
	queueInterface, exists := b.queues.Load(queue)
	if !exists {
		return nil, fmt.Errorf("queue %s does not exist", queue)
	}
	
	mq := queueInterface.(*messageQueue)
	return &QueueStats{
		Name:            mq.stats.Name,
		MessageCount:    atomic.LoadInt64(&mq.stats.MessageCount),
		ConsumerCount:   atomic.LoadInt32(&mq.stats.ConsumerCount),
		EnqueuedTotal:   atomic.LoadInt64(&mq.stats.EnqueuedTotal),
		DequeuedTotal:   atomic.LoadInt64(&mq.stats.DequeuedTotal),
		DeadLetterCount: atomic.LoadInt64(&mq.stats.DeadLetterCount),
	}, nil
}

// GetMetrics 獲取 Broker 的整體指標
func (b *SimpleBroker) GetMetrics() *Metrics {
	return b.metrics
}

// GetAllQueues 獲取所有隊列名稱
func (b *SimpleBroker) GetAllQueues() []string {
	var queues []string
	b.queues.Range(func(key, value interface{}) bool {
		queues = append(queues, key.(string))
		return true
	})
	return queues
}

// PurgeQueue 清空指定隊列
func (b *SimpleBroker) PurgeQueue(queue string) error {
	queueInterface, exists := b.queues.Load(queue)
	if !exists {
		return fmt.Errorf("queue %s does not exist", queue)
	}
	
	mq := queueInterface.(*messageQueue)
	
	// 清空隊列中的所有消息
	for {
		select {
		case <-mq.messages:
			atomic.AddInt64(&mq.stats.MessageCount, -1)
		default:
			return nil // 隊列已空
		}
	}
}

// IsHealthy 檢查 Broker 是否健康
func (b *SimpleBroker) IsHealthy() bool {
	return atomic.LoadInt32(&b.closed) == 0
}

// Close 關閉 Broker
func (b *SimpleBroker) Close() error {
	if !atomic.CompareAndSwapInt32(&b.closed, 0, 1) {
		return fmt.Errorf("broker is already closed")
	}
	
	b.cancel()
	
	// 關閉所有訂閱者通道
	b.subscribers.Range(func(key, value interface{}) bool {
		subMgr := value.(*subscriberManager)
		subMgr.mu.Lock()
		for _, subscriber := range subMgr.subscribers {
			close(subscriber)
		}
		subMgr.mu.Unlock()
		return true
	})
	
	return nil
}

// createMessageQueue 創建一個新的消息隊列
func (b *SimpleBroker) createMessageQueue(name string) *messageQueue {
	stats := &QueueStats{
		Name: name,
	}
	
	// 更新 metrics 中的隊列統計
	b.metrics.mu.Lock()
	b.metrics.QueueMetrics[name] = stats
	b.metrics.mu.Unlock()
	atomic.AddInt32(&b.metrics.ActiveQueues, 1)
	
	return &messageQueue{
		name:     name,
		messages: make(chan Message, 1000), // 1000 緩衝大小
		stats:    stats,
	}
}
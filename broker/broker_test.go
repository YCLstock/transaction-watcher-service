package broker

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewSimpleBroker(t *testing.T) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	if broker == nil {
		t.Error("Expected non-nil broker")
	}
	
	if broker.metrics == nil {
		t.Error("Expected metrics to be initialized")
	}
	
	if !broker.IsHealthy() {
		t.Error("Expected broker to be healthy")
	}
}

func TestPushPullQueue(t *testing.T) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	queueName := "test-queue"
	msg := NewMessage("msg-1", []byte("test message"), queueName)
	
	// 測試 Push
	err := broker.Push(queueName, msg)
	if err != nil {
		t.Errorf("Push failed: %v", err)
	}
	
	// 檢查統計
	stats, err := broker.GetQueueStats(queueName)
	if err != nil {
		t.Errorf("GetQueueStats failed: %v", err)
	}
	
	if stats.MessageCount != 1 {
		t.Errorf("Expected message count 1, got %d", stats.MessageCount)
	}
	
	if stats.EnqueuedTotal != 1 {
		t.Errorf("Expected enqueued total 1, got %d", stats.EnqueuedTotal)
	}
	
	// 測試 Pull
	pulledMsg, err := broker.Pull(queueName)
	if err != nil {
		t.Errorf("Pull failed: %v", err)
	}
	
	if pulledMsg == nil {
		t.Error("Expected non-nil message")
	}
	
	if pulledMsg.ID != msg.ID {
		t.Errorf("Expected message ID %s, got %s", msg.ID, pulledMsg.ID)
	}
	
	if string(pulledMsg.Body) != string(msg.Body) {
		t.Errorf("Expected message body %s, got %s", string(msg.Body), string(pulledMsg.Body))
	}
	
	// 再次檢查統計
	stats, _ = broker.GetQueueStats(queueName)
	if stats.MessageCount != 0 {
		t.Errorf("Expected message count 0 after pull, got %d", stats.MessageCount)
	}
	
	if stats.DequeuedTotal != 1 {
		t.Errorf("Expected dequeued total 1, got %d", stats.DequeuedTotal)
	}
}

func TestPullEmptyQueue(t *testing.T) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	// 從空隊列拉取消息
	msg, err := broker.Pull("non-existent-queue")
	if err == nil {
		t.Error("Expected error when pulling from non-existent queue")
	}
	
	if msg != nil {
		t.Error("Expected nil message from non-existent queue")
	}
}

func TestPullWithTimeout(t *testing.T) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	queueName := "test-timeout-queue"
	
	// 先推送一條消息以確保隊列存在
	msg := NewMessage("msg-1", []byte("test"), queueName)
	broker.Push(queueName, msg)
	
	// 清空隊列
	broker.Pull(queueName)
	
	// 測試超時
	start := time.Now()
	pulledMsg, err := broker.PullWithTimeout(queueName, 100*time.Millisecond)
	elapsed := time.Since(start)
	
	if err == nil {
		t.Error("Expected timeout error")
	}
	
	if pulledMsg != nil {
		t.Error("Expected nil message on timeout")
	}
	
	if elapsed < 90*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Errorf("Expected timeout around 100ms, got %v", elapsed)
	}
}

func TestPubSub(t *testing.T) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	topic := "test-topic"
	
	// 創建兩個訂閱者
	sub1, err := broker.Subscribe(topic)
	if err != nil {
		t.Errorf("Subscribe failed: %v", err)
	}
	
	sub2, err := broker.Subscribe(topic)
	if err != nil {
		t.Errorf("Subscribe failed: %v", err)
	}
	
	// 發布消息
	msg := NewMessage("pub-msg-1", []byte("broadcast message"), "")
	err = broker.Publish(topic, msg)
	if err != nil {
		t.Errorf("Publish failed: %v", err)
	}
	
	// 檢查兩個訂閱者都收到消息
	select {
	case receivedMsg := <-sub1:
		if receivedMsg.ID != msg.ID {
			t.Errorf("Sub1: Expected message ID %s, got %s", msg.ID, receivedMsg.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Sub1: Timeout waiting for message")
	}
	
	select {
	case receivedMsg := <-sub2:
		if receivedMsg.ID != msg.ID {
			t.Errorf("Sub2: Expected message ID %s, got %s", msg.ID, receivedMsg.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Sub2: Timeout waiting for message")
	}
	
	// 取消訂閱
	err = broker.Unsubscribe(topic, sub1)
	if err != nil {
		t.Errorf("Unsubscribe failed: %v", err)
	}
}

func TestDeadLetterQueue(t *testing.T) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	queueName := "test-dlq-queue"
	msg := NewMessage("dlq-msg-1", []byte("dlq test"), queueName)
	msg.MaxRetry = 1
	
	// 將消息移動到死信隊列
	err := broker.MoveToDLQ(queueName, msg)
	if err != nil {
		t.Errorf("MoveToDLQ failed: %v", err)
	}
	
	// 檢查死信隊列
	dlqMessages := broker.GetDLQ(queueName)
	if len(dlqMessages) != 1 {
		t.Errorf("Expected 1 message in DLQ, got %d", len(dlqMessages))
	}
	
	if dlqMessages[0].ID != msg.ID {
		t.Errorf("Expected DLQ message ID %s, got %s", msg.ID, dlqMessages[0].ID)
	}
	
	if dlqMessages[0].Attempts != 1 {
		t.Errorf("Expected attempts 1, got %d", dlqMessages[0].Attempts)
	}
	
	// 測試重新處理
	err = broker.ReprocessDLQ(queueName, msg.ID)
	if err != nil {
		t.Errorf("ReprocessDLQ failed: %v", err)
	}
	
	// 檢查死信隊列應該為空
	dlqMessages = broker.GetDLQ(queueName)
	if len(dlqMessages) != 0 {
		t.Errorf("Expected 0 messages in DLQ after reprocess, got %d", len(dlqMessages))
	}
	
	// 檢查消息是否回到原隊列
	pulledMsg, err := broker.Pull(queueName)
	if err != nil {
		t.Errorf("Pull after reprocess failed: %v", err)
	}
	
	if pulledMsg == nil {
		t.Error("Expected non-nil message after reprocess")
	}
	
	if pulledMsg.ID != msg.ID {
		t.Errorf("Expected reprocessed message ID %s, got %s", msg.ID, pulledMsg.ID)
	}
	
	if pulledMsg.Attempts != 0 {
		t.Errorf("Expected attempts reset to 0, got %d", pulledMsg.Attempts)
	}
}

func TestConcurrentAccess(t *testing.T) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	queueName := "concurrent-queue"
	numGoroutines := 10
	messagesPerGoroutine := 100
	
	var wg sync.WaitGroup
	
	// 並發推送消息
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				msg := NewMessage(
					fmt.Sprintf("worker-%d-msg-%d", workerID, j),
					[]byte(fmt.Sprintf("Message from worker %d", workerID)),
					queueName,
				)
				err := broker.Push(queueName, msg)
				if err != nil {
					t.Errorf("Concurrent push failed: %v", err)
				}
			}
		}(i)
	}
	
	wg.Wait()
	
	// 檢查統計
	stats, err := broker.GetQueueStats(queueName)
	if err != nil {
		t.Errorf("GetQueueStats failed: %v", err)
	}
	
	expectedTotal := int64(numGoroutines * messagesPerGoroutine)
	if stats.EnqueuedTotal != expectedTotal {
		t.Errorf("Expected enqueued total %d, got %d", expectedTotal, stats.EnqueuedTotal)
	}
	
	if stats.MessageCount != expectedTotal {
		t.Errorf("Expected message count %d, got %d", expectedTotal, stats.MessageCount)
	}
	
	// 並發拉取消息
	var pulledCount int64
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				msg, err := broker.Pull(queueName)
				if err != nil || msg == nil {
					return
				}
				atomic.AddInt64(&pulledCount, 1)
			}
		}()
	}
	
	wg.Wait()
	
	if pulledCount != expectedTotal {
		t.Errorf("Expected pulled count %d, got %d", expectedTotal, pulledCount)
	}
}

func TestBrokerClose(t *testing.T) {
	broker := NewSimpleBroker()
	
	if !broker.IsHealthy() {
		t.Error("Expected broker to be healthy before close")
	}
	
	err := broker.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
	
	if broker.IsHealthy() {
		t.Error("Expected broker to be unhealthy after close")
	}
	
	// 測試關閉後的操作
	msg := NewMessage("test", []byte("test"), "test")
	err = broker.Push("test", msg)
	if err == nil {
		t.Error("Expected error when pushing to closed broker")
	}
	
	_, err = broker.Pull("test")
	if err == nil {
		t.Error("Expected error when pulling from closed broker")
	}
	
	err = broker.Publish("test", msg)
	if err == nil {
		t.Error("Expected error when publishing to closed broker")
	}
	
	_, err = broker.Subscribe("test")
	if err == nil {
		t.Error("Expected error when subscribing to closed broker")
	}
}

func TestGetAllQueues(t *testing.T) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	// 初始應該沒有隊列
	queues := broker.GetAllQueues()
	if len(queues) != 0 {
		t.Errorf("Expected 0 queues initially, got %d", len(queues))
	}
	
	// 創建幾個隊列
	queueNames := []string{"queue1", "queue2", "queue3"}
	for _, name := range queueNames {
		msg := NewMessage("test", []byte("test"), name)
		broker.Push(name, msg)
	}
	
	queues = broker.GetAllQueues()
	if len(queues) != len(queueNames) {
		t.Errorf("Expected %d queues, got %d", len(queueNames), len(queues))
	}
	
	// 檢查所有隊列都存在
	queueSet := make(map[string]bool)
	for _, queue := range queues {
		queueSet[queue] = true
	}
	
	for _, expectedQueue := range queueNames {
		if !queueSet[expectedQueue] {
			t.Errorf("Expected queue %s not found", expectedQueue)
		}
	}
}

func TestPurgeQueue(t *testing.T) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	queueName := "purge-test-queue"
	
	// 推送一些消息
	for i := 0; i < 5; i++ {
		msg := NewMessage(fmt.Sprintf("msg-%d", i), []byte("test"), queueName)
		broker.Push(queueName, msg)
	}
	
	// 檢查消息數量
	stats, _ := broker.GetQueueStats(queueName)
	if stats.MessageCount != 5 {
		t.Errorf("Expected 5 messages before purge, got %d", stats.MessageCount)
	}
	
	// 清空隊列
	err := broker.PurgeQueue(queueName)
	if err != nil {
		t.Errorf("PurgeQueue failed: %v", err)
	}
	
	// 檢查隊列是否為空
	stats, _ = broker.GetQueueStats(queueName)
	if stats.MessageCount != 0 {
		t.Errorf("Expected 0 messages after purge, got %d", stats.MessageCount)
	}
}
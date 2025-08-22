package broker

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func BenchmarkBrokerPush(b *testing.B) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	queueName := "benchmark-push-queue"
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			msg := NewMessage(fmt.Sprintf("msg-%d", i), []byte("benchmark message"), queueName)
			broker.Push(queueName, msg)
			i++
		}
	})
}

func BenchmarkBrokerPull(b *testing.B) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	queueName := "benchmark-pull-queue"
	
	// 預先填充隊列
	for i := 0; i < b.N; i++ {
		msg := NewMessage(fmt.Sprintf("msg-%d", i), []byte("benchmark message"), queueName)
		broker.Push(queueName, msg)
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			broker.Pull(queueName)
		}
	})
}

func BenchmarkBrokerPushPull(b *testing.B) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	queueName := "benchmark-pushpull-queue"
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// Push
			msg := NewMessage(fmt.Sprintf("msg-%d", i), []byte("benchmark message"), queueName)
			broker.Push(queueName, msg)
			
			// Pull
			broker.Pull(queueName)
			i++
		}
	})
}

func BenchmarkBrokerPublish(b *testing.B) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	topic := "benchmark-topic"
	
	// 創建一些訂閱者
	for i := 0; i < 10; i++ {
		broker.Subscribe(topic)
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			msg := NewMessage(fmt.Sprintf("msg-%d", i), []byte("benchmark message"), "")
			broker.Publish(topic, msg)
			i++
		}
	})
}

func BenchmarkBrokerConcurrentQueues(b *testing.B) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	numQueues := 100
	queues := make([]string, numQueues)
	for i := 0; i < numQueues; i++ {
		queues[i] = fmt.Sprintf("queue-%d", i)
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			queueName := queues[i%numQueues]
			msg := NewMessage(fmt.Sprintf("msg-%d", i), []byte("benchmark message"), queueName)
			broker.Push(queueName, msg)
			broker.Pull(queueName)
			i++
		}
	})
}

// 延遲測試
func BenchmarkBrokerLatency(b *testing.B) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	queueName := "latency-test-queue"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		
		msg := NewMessage(fmt.Sprintf("msg-%d", i), []byte("latency test message"), queueName)
		broker.Push(queueName, msg)
		broker.Pull(queueName)
		
		_ = time.Since(start)
	}
}

// 高併發測試
func BenchmarkBrokerHighConcurrency(b *testing.B) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	numWorkers := 1000
	queueName := "high-concurrency-queue"
	
	var wg sync.WaitGroup
	
	b.ResetTimer()
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < b.N/numWorkers; j++ {
				msg := NewMessage(fmt.Sprintf("worker-%d-msg-%d", workerID, j), []byte("concurrency test"), queueName)
				broker.Push(queueName, msg)
			}
		}(i)
	}
	
	wg.Wait()
	
	// 測試併發拉取
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < b.N/numWorkers; j++ {
				broker.Pull(queueName)
			}
		}()
	}
	
	wg.Wait()
}

// Memory footprint 測試
func BenchmarkBrokerMemory(b *testing.B) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	queueName := "memory-test-queue"
	largePayload := make([]byte, 1024) // 1KB payload
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := NewMessage(fmt.Sprintf("msg-%d", i), largePayload, queueName)
		broker.Push(queueName, msg)
	}
	
	// 清理
	for i := 0; i < b.N; i++ {
		broker.Pull(queueName)
	}
}

// TPS 測試（每秒事務數）
func BenchmarkBrokerTPS(b *testing.B) {
	broker := NewSimpleBroker()
	defer broker.Close()
	
	queueName := "tps-test-queue"
	duration := 5 * time.Second
	
	// 預熱
	for i := 0; i < 1000; i++ {
		msg := NewMessage(fmt.Sprintf("warmup-%d", i), []byte("warmup"), queueName)
		broker.Push(queueName, msg)
		broker.Pull(queueName)
	}
	
	b.ResetTimer()
	
	start := time.Now()
	var ops int64
	
	done := make(chan bool)
	go func() {
		time.Sleep(duration)
		done <- true
	}()
	
	for {
		select {
		case <-done:
			elapsed := time.Since(start)
			tps := float64(ops) / elapsed.Seconds()
			b.Logf("TPS: %.2f, Total Operations: %d, Duration: %v", tps, ops, elapsed)
			return
		default:
			msg := NewMessage(fmt.Sprintf("tps-msg-%d", ops), []byte("tps test"), queueName)
			broker.Push(queueName, msg)
			broker.Pull(queueName)
			ops++
		}
	}
}
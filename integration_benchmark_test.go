package main

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/YCLstock/transaction-watcher/broker"
)

func BenchmarkEndToEndFlow(b *testing.B) {
	// 初始化全局變量
	messageBroker = broker.NewSimpleBroker()
	defer messageBroker.Close()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// 創建區塊消息
			blockMsg := BlockMessage{
				BlockNumber: string(rune(i)),
				BlockHash:   generateMessageID(),
				Timestamp:   time.Now(),
				TxCount:     1,
				Transactions: []TransactionInfo{
					{
						Hash:     generateMessageID(),
						To:       targetAddress,
						From:     "0xtest",
						Value:    "1000000000000000000",
						GasPrice: "20000000000",
					},
				},
			}
			
			// 序列化並推送
			blockMsgData, _ := json.Marshal(blockMsg)
			msg := broker.NewMessage(generateMessageID(), blockMsgData, "blocks")
			messageBroker.Push("blocks", msg)
			
			// 拉取並處理
			pulledMsg, _ := messageBroker.Pull("blocks")
			if pulledMsg != nil {
				var pulledBlockMsg BlockMessage
				json.Unmarshal(pulledMsg.Body, &pulledBlockMsg)
				
				// 處理交易
				for _, tx := range pulledBlockMsg.Transactions {
					txMsgData, _ := json.Marshal(tx)
					txMsg := broker.NewMessage(generateMessageID(), txMsgData, "transactions")
					messageBroker.Push("transactions", txMsg)
					messageBroker.Pull("transactions")
				}
			}
			i++
		}
	})
}

func BenchmarkHTTPEndpoints(b *testing.B) {
	// 初始化全局變量
	messageBroker = broker.NewSimpleBroker()
	defer messageBroker.Close()
	startTime = time.Now()
	
	// 預先填充一些數據
	for i := 0; i < 100; i++ {
		msg := broker.NewMessage(generateMessageID(), []byte("test"), "test-queue")
		messageBroker.Push("test-queue", msg)
	}
	
	b.ResetTimer()
	b.Run("Health", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			metrics := messageBroker.GetMetrics().GetStats()
			_ = metrics
		}
	})
	
	b.Run("Metrics", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			stats := messageBroker.GetMetrics().GetStats()
			_ = stats
		}
	})
	
	b.Run("Queues", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			queues := messageBroker.GetAllQueues()
			for _, queueName := range queues {
				messageBroker.GetQueueStats(queueName)
			}
		}
	})
}

func BenchmarkConcurrentWorkers(b *testing.B) {
	messageBroker = broker.NewSimpleBroker()
	defer messageBroker.Close()
	
	const numWorkers = 10
	const messagesPerWorker = 1000
	
	b.ResetTimer()
	
	// 生產者：推送消息
	go func() {
		for i := 0; i < b.N; i++ {
			blockMsg := BlockMessage{
				BlockNumber: string(rune(i)),
				BlockHash:   generateMessageID(),
				Timestamp:   time.Now(),
				TxCount:     1,
				Transactions: []TransactionInfo{
					{
						Hash:  generateMessageID(),
						To:    targetAddress,
						Value: "1000000000000000000",
					},
				},
			}
			
			blockMsgData, _ := json.Marshal(blockMsg)
			msg := broker.NewMessage(generateMessageID(), blockMsgData, "blocks")
			messageBroker.Push("blocks", msg)
		}
	}()
	
	// 消費者：多個 worker 並發處理
	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			processed := 0
			for processed < b.N/numWorkers {
				msg, err := messageBroker.PullWithTimeout("blocks", 10*time.Millisecond)
				if err != nil || msg == nil {
					continue
				}
				
				var blockMsg BlockMessage
				if json.Unmarshal(msg.Body, &blockMsg) == nil {
					// 處理區塊中的交易
					for _, tx := range blockMsg.Transactions {
						txData, _ := json.Marshal(tx)
						txMsg := broker.NewMessage(generateMessageID(), txData, "transactions")
						messageBroker.Push("transactions", txMsg)
					}
					processed++
				}
			}
		}(w)
	}
	
	wg.Wait()
}

func BenchmarkMessageThroughput(b *testing.B) {
	messageBroker = broker.NewSimpleBroker()
	defer messageBroker.Close()
	
	// 測試純消息吞吐量
	duration := 1 * time.Second
	
	b.ResetTimer()
	
	start := time.Now()
	var operations int64
	
	done := make(chan bool)
	go func() {
		time.Sleep(duration)
		done <- true
	}()
	
	for {
		select {
		case <-done:
			elapsed := time.Since(start)
			tps := float64(operations) / elapsed.Seconds()
			b.Logf("整合系統 TPS: %.2f, 總操作數: %d, 耗時: %v", tps, operations, elapsed)
			return
		default:
			// 創建並處理一個完整的區塊消息
			blockMsg := BlockMessage{
				BlockNumber: string(rune(operations)),
				BlockHash:   generateMessageID(),
				Timestamp:   time.Now(),
				TxCount:     1,
				Transactions: []TransactionInfo{
					{
						Hash:  generateMessageID(),
						To:    targetAddress,
						Value: "1000000000000000000",
					},
				},
			}
			
			blockMsgData, _ := json.Marshal(blockMsg)
			msg := broker.NewMessage(generateMessageID(), blockMsgData, "blocks")
			messageBroker.Push("blocks", msg)
			
			pulledMsg, _ := messageBroker.Pull("blocks")
			if pulledMsg != nil {
				var pulledBlockMsg BlockMessage
				json.Unmarshal(pulledMsg.Body, &pulledBlockMsg)
				
				for _, tx := range pulledBlockMsg.Transactions {
					txData, _ := json.Marshal(tx)
					txMsg := broker.NewMessage(generateMessageID(), txData, "transactions")
					messageBroker.Push("transactions", txMsg)
					messageBroker.Pull("transactions")
				}
			}
			operations++
		}
	}
}

func BenchmarkMemoryUsage(b *testing.B) {
	messageBroker = broker.NewSimpleBroker()
	defer messageBroker.Close()
	
	// 大量消息的內存使用測試
	largePayload := make([]byte, 1024) // 1KB per message
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blockMsg := BlockMessage{
			BlockNumber: string(rune(i)),
			BlockHash:   generateMessageID(),
			Timestamp:   time.Now(),
			TxCount:     1,
		}
		
		// 添加大 payload
		blockMsgData := append(largePayload, []byte(blockMsg.BlockNumber)...)
		msg := broker.NewMessage(generateMessageID(), blockMsgData, "memory-test")
		messageBroker.Push("memory-test", msg)
	}
	
	// 清理
	for i := 0; i < b.N; i++ {
		messageBroker.Pull("memory-test")
	}
}
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/YCLstock/transaction-watcher/broker"
)

func TestGenerateMessageID(t *testing.T) {
	id1 := generateMessageID()
	id2 := generateMessageID()
	
	if id1 == id2 {
		t.Error("Expected unique message IDs")
	}
	
	if len(id1) != 32 { // 16 bytes * 2 hex chars
		t.Errorf("Expected 32 character ID, got %d", len(id1))
	}
}

func TestBlockMessageSerialization(t *testing.T) {
	blockMsg := BlockMessage{
		BlockNumber: "12345",
		BlockHash:   "0xabcdef",
		Timestamp:   time.Now(),
		TxCount:     2,
		Transactions: []TransactionInfo{
			{
				Hash:     "0x123",
				To:       targetAddress,
				From:     "0xfrom",
				Value:    "1000000000000000000",
				GasPrice: "20000000000",
			},
		},
	}
	
	// 序列化
	data, err := json.Marshal(blockMsg)
	if err != nil {
		t.Errorf("Serialization failed: %v", err)
	}
	
	// 反序列化
	var deserializedMsg BlockMessage
	err = json.Unmarshal(data, &deserializedMsg)
	if err != nil {
		t.Errorf("Deserialization failed: %v", err)
	}
	
	if deserializedMsg.BlockNumber != blockMsg.BlockNumber {
		t.Errorf("Expected block number %s, got %s", blockMsg.BlockNumber, deserializedMsg.BlockNumber)
	}
	
	if len(deserializedMsg.Transactions) != len(blockMsg.Transactions) {
		t.Errorf("Expected %d transactions, got %d", len(blockMsg.Transactions), len(deserializedMsg.Transactions))
	}
}

func TestHTTPHealthEndpoint(t *testing.T) {
	// 初始化全局變量
	messageBroker = broker.NewSimpleBroker()
	defer messageBroker.Close()
	startTime = time.Now()
	
	req, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(handleHealth)
	handler.ServeHTTP(rr, req)
	
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, status)
	}
	
	var health map[string]interface{}
	err = json.Unmarshal(rr.Body.Bytes(), &health)
	if err != nil {
		t.Errorf("Failed to parse JSON response: %v", err)
	}
	
	if health["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %s", health["status"])
	}
	
	if _, exists := health["uptime"]; !exists {
		t.Error("Expected uptime field in response")
	}
	
	if health["broker"] != true {
		t.Errorf("Expected broker to be healthy")
	}
}

func TestHTTPMetricsEndpoint(t *testing.T) {
	// 初始化全局變量
	messageBroker = broker.NewSimpleBroker()
	defer messageBroker.Close()
	startTime = time.Now()
	
	// 推送一些測試消息來產生 metrics
	msg := broker.NewMessage("test-1", []byte("test"), "test-queue")
	messageBroker.Push("test-queue", msg)
	messageBroker.Pull("test-queue")
	
	req, err := http.NewRequest("GET", "/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}
	
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(handleMetrics)
	handler.ServeHTTP(rr, req)
	
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, status)
	}
	
	body := rr.Body.String()
	
	// 檢查 Prometheus 格式的輸出
	if !bytes.Contains(rr.Body.Bytes(), []byte("messages_total")) {
		t.Error("Expected messages_total metric in response")
	}
	
	if !bytes.Contains(rr.Body.Bytes(), []byte("uptime_seconds")) {
		t.Error("Expected uptime_seconds metric in response")
	}
	
	// 檢查數值
	if !bytes.Contains(rr.Body.Bytes(), []byte("messages_total 1")) {
		t.Error("Expected messages_total to be 1")
	}
	
	t.Logf("Metrics response:\n%s", body)
}

func TestHTTPQueuesEndpoint(t *testing.T) {
	// 初始化全局變量
	messageBroker = broker.NewSimpleBroker()
	defer messageBroker.Close()
	startTime = time.Now()
	
	// 創建一些隊列
	msg1 := broker.NewMessage("test-1", []byte("test"), "queue1")
	msg2 := broker.NewMessage("test-2", []byte("test"), "queue2")
	messageBroker.Push("queue1", msg1)
	messageBroker.Push("queue2", msg2)
	
	req, err := http.NewRequest("GET", "/queues", nil)
	if err != nil {
		t.Fatal(err)
	}
	
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(handleQueues)
	handler.ServeHTTP(rr, req)
	
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, status)
	}
	
	var queues map[string]interface{}
	err = json.Unmarshal(rr.Body.Bytes(), &queues)
	if err != nil {
		t.Errorf("Failed to parse JSON response: %v", err)
	}
	
	if len(queues) != 2 {
		t.Errorf("Expected 2 queues, got %d", len(queues))
	}
	
	if _, exists := queues["queue1"]; !exists {
		t.Error("Expected queue1 in response")
	}
	
	if _, exists := queues["queue2"]; !exists {
		t.Error("Expected queue2 in response")
	}
}

func TestHTTPDLQEndpoint(t *testing.T) {
	// 初始化全局變量
	messageBroker = broker.NewSimpleBroker()
	defer messageBroker.Close()
	startTime = time.Now()
	
	// 創建死信消息
	msg := broker.NewMessage("dlq-test", []byte("failed message"), "test-queue")
	messageBroker.MoveToDLQ("test-queue", msg)
	
	// 測試正常請求
	req, err := http.NewRequest("GET", "/dlq?queue=test-queue", nil)
	if err != nil {
		t.Fatal(err)
	}
	
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(handleDLQ)
	handler.ServeHTTP(rr, req)
	
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, status)
	}
	
	var response map[string]interface{}
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	if err != nil {
		t.Errorf("Failed to parse JSON response: %v", err)
	}
	
	if response["queue"] != "test-queue" {
		t.Errorf("Expected queue 'test-queue', got %s", response["queue"])
	}
	
	if response["count"].(float64) != 1 {
		t.Errorf("Expected count 1, got %f", response["count"])
	}
	
	// 測試缺少 queue 參數
	req2, _ := http.NewRequest("GET", "/dlq", nil)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	
	if status := rr2.Code; status != http.StatusBadRequest {
		t.Errorf("Expected status code %d for missing queue param, got %d", http.StatusBadRequest, status)
	}
}

func TestBrokerIntegrationFlow(t *testing.T) {
	// 初始化全局變量
	messageBroker = broker.NewSimpleBroker()
	defer messageBroker.Close()
	
	// 模擬完整的消息流
	blockMsg := BlockMessage{
		BlockNumber: "12345",
		BlockHash:   "0xabcdef",
		Timestamp:   time.Now(),
		TxCount:     1,
		Transactions: []TransactionInfo{
			{
				Hash:     "0x123456",
				To:       targetAddress,
				From:     "0xfrom123",
				Value:    "1000000000000000000", // 1 ETH
				GasPrice: "20000000000",
			},
		},
	}
	
	// 序列化區塊消息
	blockMsgData, err := json.Marshal(blockMsg)
	if err != nil {
		t.Fatalf("Failed to marshal block message: %v", err)
	}
	
	// 創建並推送區塊消息
	msg := broker.NewMessage(generateMessageID(), blockMsgData, "blocks")
	err = messageBroker.Push("blocks", msg)
	if err != nil {
		t.Fatalf("Failed to push block message: %v", err)
	}
	
	// 拉取並處理區塊消息
	pulledMsg, err := messageBroker.Pull("blocks")
	if err != nil {
		t.Fatalf("Failed to pull block message: %v", err)
	}
	
	if pulledMsg == nil {
		t.Fatal("Expected non-nil pulled message")
	}
	
	// 反序列化並驗證
	var pulledBlockMsg BlockMessage
	err = json.Unmarshal(pulledMsg.Body, &pulledBlockMsg)
	if err != nil {
		t.Fatalf("Failed to unmarshal pulled message: %v", err)
	}
	
	if pulledBlockMsg.BlockNumber != blockMsg.BlockNumber {
		t.Errorf("Expected block number %s, got %s", blockMsg.BlockNumber, pulledBlockMsg.BlockNumber)
	}
	
	if len(pulledBlockMsg.Transactions) != 1 {
		t.Errorf("Expected 1 transaction, got %d", len(pulledBlockMsg.Transactions))
	}
	
	// 檢查目標交易
	tx := pulledBlockMsg.Transactions[0]
	if tx.To != targetAddress {
		t.Errorf("Expected target address %s, got %s", targetAddress, tx.To)
	}
	
	// 模擬處理交易消息
	txMsgData, _ := json.Marshal(tx)
	txMsg := broker.NewMessage(generateMessageID(), txMsgData, "transactions")
	err = messageBroker.Push("transactions", txMsg)
	if err != nil {
		t.Fatalf("Failed to push transaction message: %v", err)
	}
	
	// 驗證交易隊列
	pulledTxMsg, err := messageBroker.Pull("transactions")
	if err != nil {
		t.Fatalf("Failed to pull transaction message: %v", err)
	}
	
	var pulledTxInfo TransactionInfo
	err = json.Unmarshal(pulledTxMsg.Body, &pulledTxInfo)
	if err != nil {
		t.Fatalf("Failed to unmarshal transaction message: %v", err)
	}
	
	if pulledTxInfo.Hash != tx.Hash {
		t.Errorf("Expected tx hash %s, got %s", tx.Hash, pulledTxInfo.Hash)
	}
	
	// 檢查 broker 統計
	metrics := messageBroker.GetMetrics().GetStats()
	if metrics["total_messages"].(int64) < 2 {
		t.Errorf("Expected at least 2 total messages, got %d", metrics["total_messages"])
	}
	
	if metrics["processed_messages"].(int64) < 2 {
		t.Errorf("Expected at least 2 processed messages, got %d", metrics["processed_messages"])
	}
}

func TestWorkerPoolIntegration(t *testing.T) {
	// 初始化全局變量
	messageBroker = broker.NewSimpleBroker()
	defer messageBroker.Close()
	
	// 模擬多個區塊消息
	numBlocks := 10
	for i := 0; i < numBlocks; i++ {
		blockMsg := BlockMessage{
			BlockNumber: string(rune(12345 + i)),
			BlockHash:   generateMessageID(),
			Timestamp:   time.Now(),
			TxCount:     1,
			Transactions: []TransactionInfo{
				{
					Hash:     generateMessageID(),
					To:       targetAddress,
					From:     "0xfrom" + string(rune(i)),
					Value:    "1000000000000000000",
					GasPrice: "20000000000",
				},
			},
		}
		
		blockMsgData, _ := json.Marshal(blockMsg)
		msg := broker.NewMessage(generateMessageID(), blockMsgData, "blocks")
		messageBroker.Push("blocks", msg)
	}
	
	// 檢查消息數量
	stats, err := messageBroker.GetQueueStats("blocks")
	if err != nil {
		t.Fatalf("Failed to get queue stats: %v", err)
	}
	
	if stats.MessageCount != int64(numBlocks) {
		t.Errorf("Expected %d messages in blocks queue, got %d", numBlocks, stats.MessageCount)
	}
	
	// 模擬 worker 處理
	processedCount := 0
	for i := 0; i < numBlocks; i++ {
		msg, err := messageBroker.Pull("blocks")
		if err != nil || msg == nil {
			break
		}
		processedCount++
	}
	
	if processedCount != numBlocks {
		t.Errorf("Expected to process %d messages, got %d", numBlocks, processedCount)
	}
}
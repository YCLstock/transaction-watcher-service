package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/YCLstock/transaction-watcher/broker"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

// 全局變數
const targetAddress = "0x7AF963CF6D228E564E2A0AA0DDBF06210B38615D"

var (
	messageBroker broker.Broker
	startTime     time.Time
)

// BlockMessage 代表區塊訊息的結構
type BlockMessage struct {
	BlockNumber string            `json:"block_number"`
	BlockHash   string            `json:"block_hash"`
	Timestamp   time.Time         `json:"timestamp"`
	TxCount     int               `json:"tx_count"`
	Transactions []TransactionInfo `json:"transactions,omitempty"`
}

// TransactionInfo 代表交易資訊
type TransactionInfo struct {
	Hash     string `json:"hash"`
	To       string `json:"to"`
	From     string `json:"from"`
	Value    string `json:"value"`
	GasPrice string `json:"gas_price"`
}

// generateMessageID 生成唯一的消息ID
func generateMessageID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// startHTTPServer 啟動 HTTP API 服務器
func startHTTPServer() {
	http.HandleFunc("/metrics", handleMetrics)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/queues", handleQueues)
	http.HandleFunc("/dlq", handleDLQ)

	logrus.Info("🌐 HTTP API 服務器已啟動: http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logrus.WithError(err).Error("HTTP 服務器啟動失敗")
	}
}

// handleMetrics 處理 /metrics 端點 (Prometheus 格式)
func handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	metrics := messageBroker.GetMetrics().GetStats()
	
	fmt.Fprintf(w, "# HELP messages_total Total messages processed\n")
	fmt.Fprintf(w, "# TYPE messages_total counter\n")
	fmt.Fprintf(w, "messages_total %d\n", metrics["total_messages"])
	
	fmt.Fprintf(w, "# HELP messages_processed_total Total messages processed successfully\n")
	fmt.Fprintf(w, "# TYPE messages_processed_total counter\n")
	fmt.Fprintf(w, "messages_processed_total %d\n", metrics["processed_messages"])
	
	fmt.Fprintf(w, "# HELP messages_failed_total Total messages failed\n")
	fmt.Fprintf(w, "# TYPE messages_failed_total counter\n")
	fmt.Fprintf(w, "messages_failed_total %d\n", metrics["failed_messages"])
	
	fmt.Fprintf(w, "# HELP active_queues Number of active queues\n")
	fmt.Fprintf(w, "# TYPE active_queues gauge\n")
	fmt.Fprintf(w, "active_queues %d\n", metrics["active_queues"])
	
	fmt.Fprintf(w, "# HELP uptime_seconds Uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE uptime_seconds counter\n")
	fmt.Fprintf(w, "uptime_seconds %.2f\n", metrics["uptime_seconds"])
}

// handleHealth 處理 /health 端點
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	health := map[string]interface{}{
		"status":     "healthy",
		"uptime":     time.Since(startTime).Seconds(),
		"broker":     messageBroker.IsHealthy(),
		"queues":     len(messageBroker.GetAllQueues()),
		"timestamp":  time.Now(),
	}
	
	json.NewEncoder(w).Encode(health)
}

// handleQueues 處理 /queues 端點
func handleQueues(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	queues := make(map[string]interface{})
	for _, queueName := range messageBroker.GetAllQueues() {
		stats, err := messageBroker.GetQueueStats(queueName)
		if err == nil {
			queues[queueName] = stats
		}
	}
	
	json.NewEncoder(w).Encode(queues)
}

// handleDLQ 處理 /dlq 端點
func handleDLQ(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	queueName := r.URL.Query().Get("queue")
	if queueName == "" {
		http.Error(w, "queue parameter is required", http.StatusBadRequest)
		return
	}
	
	dlqMessages := messageBroker.GetDLQ(queueName)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"queue":    queueName,
		"messages": dlqMessages,
		"count":    len(dlqMessages),
	})
}

// startWatching 函式包含了我們所有的核心監聽邏輯
func startWatching() {
	// 從環境變數讀取 WSS URL
	wssURL := os.Getenv("ALCHEMY_WSS_URL")
	if wssURL == "" {
		logrus.Fatal("❌ 環境變數 ALCHEMY_WSS_URL 未設定，請設定您的 Alchemy WebSocket URL")
		return
	}

	logrus.WithFields(logrus.Fields{
		"targetAddress": targetAddress,
	}).Info("🎯 正在啟動監聽器...")

	client, err := ethclient.Dial(wssURL)
	if err != nil {
		logrus.WithError(err).Error("❌ WebSocket 連線失敗")
		return
	}
	defer client.Close()
	logrus.Info("🎉 WebSocket 連線成功！")

	headers := make(chan *types.Header)
	sub, err := client.SubscribeNewHead(context.Background(), headers)
	if err != nil {
		logrus.WithError(err).Error("❌ 訂閱新區塊事件失敗")
		return
	}
	logrus.Info("✅ 訂閱成功！正在等待新的區塊...")

	// --- 使用 Message Broker 處理區塊 ---
	const numWorkers = 4
	const blockQueueName = "blocks"
	const transactionQueueName = "transactions"

	// 啟動 Worker Pool 從 Broker 消費消息
	for i := 1; i <= numWorkers; i++ {
		go func(workerID int) {
			for {
				// 從區塊隊列拉取消息
				blockMsg, err := messageBroker.PullWithTimeout(blockQueueName, 1*time.Second)
				if err != nil || blockMsg == nil {
					continue
				}

				// 解析區塊消息
				var blockMessage BlockMessage
				if err := json.Unmarshal(blockMsg.Body, &blockMessage); err != nil {
					logrus.WithError(err).Warn("⚠️ 解析區塊消息失敗")
					continue
				}

				logrus.WithFields(logrus.Fields{
					"workerID":    workerID,
					"blockNumber": blockMessage.BlockNumber,
					"txCount":     blockMessage.TxCount,
				}).Debug("🛠️ 工人開始處理區塊")

				// 從消息中獲取區塊信息 (已預處理)
				blockNumber := blockMessage.BlockNumber
				
				// 處理交易 (如果有目標交易)
				for _, txInfo := range blockMessage.Transactions {
					if strings.EqualFold(txInfo.To, targetAddress) {
						// 發現目標交易，推送到交易隊列進行進一步處理
						txMsgData, _ := json.Marshal(txInfo)
						txMsg := broker.NewMessage(
							generateMessageID(),
							txMsgData,
							transactionQueueName,
						)
						
						messageBroker.Push(transactionQueueName, txMsg)
						
						logrus.WithFields(logrus.Fields{
							"blockNumber": blockNumber,
							"txHash":      txInfo.Hash,
							"to":          txInfo.To,
							"valueWei":    txInfo.Value,
							"workerID":    workerID,
						}).Info("🚨🚨🚨 偵測到目標存款！")
					}
				}
			}
		}(i)
	}

	// 主迴圈：接收新區塊並發送到隊列
	for {
		select {
		case err := <-sub.Err():
			logrus.WithError(err).Error("😥 訂閱連線中斷")
			// Broker 會自動處理清理，無需手動關閉
			return              // 返回後，main 函式的迴圈會讓我們重試

		case header := <-headers:
			// 收到新區塊，立刻發送到處理隊列，不阻塞
			// 創建區塊消息並推送到 Broker
			block, err := client.BlockByHash(context.Background(), header.Hash())
			if err != nil {
				logrus.WithError(err).Warn("⚠️ 獲取區塊詳情失敗")
				continue
			}
			
			var transactions []TransactionInfo
			for _, tx := range block.Transactions() {
				if tx.To() != nil && strings.EqualFold(tx.To().Hex(), targetAddress) {
					// 只包含目標地址的交易
					txInfo := TransactionInfo{
						Hash:     tx.Hash().Hex(),
						To:       tx.To().Hex(),
						Value:    tx.Value().String(),
						GasPrice: tx.GasPrice().String(),
					}
					// 簡化處理，不獲取 from 地址（需要簽名信息）
					txInfo.From = "unknown"
					transactions = append(transactions, txInfo)
				}
			}
			
			blockMessage := BlockMessage{
				BlockNumber:  header.Number.String(),
				BlockHash:    header.Hash().Hex(),
				Timestamp:    time.Now(),
				TxCount:      len(block.Transactions()),
				Transactions: transactions,
			}
			
			blockMsgData, _ := json.Marshal(blockMessage)
			msg := broker.NewMessage(
				generateMessageID(),
				blockMsgData,
				blockQueueName,
			)
			
			err = messageBroker.Push(blockQueueName, msg)
			if err != nil {
				logrus.WithField("blockNumber", header.Number.String()).WithError(err).Warn("⚠️ 推送區塊到隊列失敗！")
			}
		}
	}
}

func main() {
	// 在程式啟動時，從 .env 檔案載入環境變數
	err := godotenv.Load()
	if err != nil {
		logrus.Warn("⚠️ 找不到 .env 檔案，將會直接使用環境變數")
	}

	// 記錄啟動時間
	startTime = time.Now()
	
	// 初始化 Message Broker
	messageBroker = broker.NewSimpleBroker()
	defer messageBroker.Close()
	
	logrus.Info("🚀 高性能 Message Broker 已啟動")
	logrus.WithFields(logrus.Fields{
		"target_address": targetAddress,
		"broker_type":   "SimpleBroker",
	}).Info("🎯 區塊鏈交易監聽服務已啟動")
	
	// 啟動 HTTP API 服務器
	go startHTTPServer()

	// --- 這是我們的「永動機」和「錯誤重試」核心 ---
	for {
		startWatching() // 啟動監聽器

		// 如果 startWatching 因為任何錯誤而返回，我們會在這裡等待 15 秒
		logrus.Warn("監聽器已停止，將在 15 秒後嘗試重啟...")
		time.Sleep(15 * time.Second)
	}
}

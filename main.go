package main

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

// 全局變數
const targetAddress = "0x7AF963CF6D228E564E2A0AA0DDBF06210B38615D"

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
		// 注意：這裡我們用 logrus.Error，而不是 log.Fatalf，這樣程式不會直接退出
		logrus.WithError(err).Error("❌ WebSocket 連線失敗")
		return // 返回後，main 函式的迴圈會讓我們重試
	}
	defer client.Close() // 確保函式結束時關閉連線
	logrus.Info("🎉 WebSocket 連線成功！")

	headers := make(chan *types.Header)
	sub, err := client.SubscribeNewHead(context.Background(), headers)
	if err != nil {
		logrus.WithError(err).Error("❌ 訂閱新區塊事件失敗")
		return
	}
	logrus.Info("✅ 訂閱成功！正在等待新的區塊...")

	for {
		select {
		// 如果訂閱本身出錯（例如 Alchemy 斷開連線）
		case err := <-sub.Err():
			logrus.WithError(err).Error("😥 訂閱連線中斷")
			return // 返回後，main 函式的迴圈會讓我們重試

		// 收到新區塊
		case header := <-headers:
			block, err := client.BlockByHash(context.Background(), header.Hash())
			if err != nil {
				logrus.WithField("block", header.Number.String()).WithError(err).Warn("⚠️ 獲取區塊資訊失敗")
				continue
			}

			for _, tx := range block.Transactions() {
				if tx.To() != nil && strings.EqualFold(tx.To().Hex(), targetAddress) {
					// 使用 logrus 的結構化日誌，將關鍵資訊記錄下來
					logrus.WithFields(logrus.Fields{
						"blockNumber": block.Number().String(),
						"txHash":      tx.Hash().Hex(),
						"to":          tx.To().Hex(),
						"valueWei":    tx.Value().String(),
					}).Info("🚨🚨🚨 偵測到目標存款！")
				}
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

	// --- 這是我們的「永動機」和「錯誤重試」核心 ---
	for {
		startWatching() // 啟動監聽器

		// 如果 startWatching 因為任何錯誤而返回，我們會在這裡等待 15 秒
		logrus.Warn("監聽器已停止，將在 15 秒後嘗試重啟...")
		time.Sleep(15 * time.Second)
	}
}

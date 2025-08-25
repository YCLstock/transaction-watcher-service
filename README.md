# High-Performance Blockchain Transaction Watcher

A production-ready Go application that monitors Ethereum blockchain transactions in real-time. It features a self-built, high-performance, in-memory message broker (41,000+ TPS) designed for high-throughput, low-latency systems.

## 🏗️ System Architecture

The system uses a producer-consumer model built around a high-performance message broker, ensuring asynchronous processing and scalability.

```
Ethereum WebSocket → Block Queue → Worker Pool → Transaction Queue → Processing
                         ↓
                    Message Broker
                    ├── Queue Mode (Point-to-Point)
                    ├── Pub/Sub Mode (Broadcast)  
                    ├── Dead Letter Queue
                    └── Metrics & Monitoring
                         ↓
                    HTTP API (:8080)
                    ├── /health
                    ├── /metrics (Prometheus)
                    ├── /queues
                    └── /dlq
```

## ✨ Features

*   **High-Performance Broker**: 41,000+ TPS in-memory message broker with zero external dependencies.
*   **Concurrent Worker Pool**: Parallel processing of blockchain data to maximize CPU usage.
*   **Backpressure Control**: Buffered channels prevent memory overflow under high load.
*   **Dead Letter Queue (DLQ)**: Automatically handles and isolates failed messages for later inspection.
*   **Pub/Sub & Queueing**: Supports both broadcast (Pub/Sub) and point-to-point (Queue) messaging patterns.
*   **Rich Observability**: Production-ready monitoring via Prometheus metrics and health check endpoints.
*   **Atomic & Lock-Free**: Utilizes `sync.Map` and atomic operations for thread-safe, high-concurrency performance.
*   **Auto-Recovery**: Graceful error handling and automatic reconnection logic.

## 📊 Performance

| Metric | Value |
|--------|--------|
| **Pure Broker TPS** | 41,234 ops/sec |
| **End-to-End System TPS** | 16,148 ops/sec |
| **Latency P99** | < 1ms |
| **Test Coverage** | 88.4% |

## 🛠️ Technology Stack

- **Language**: Go 1.25+
- **Blockchain**: Ethereum (via go-ethereum)
- **Concurrency**: Goroutines, Channels, `sync.Map`, Atomic Operations
- **Monitoring**: Prometheus metrics, HTTP APIs
- **Testing**: Unit Tests, Integration Tests, Benchmarks

## 📦 Installation

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/YCLstock/transaction-watcher-service.git
    cd transaction-watcher-service
    ```

2.  **Install dependencies:**
    ```bash
    go mod tidy
    ```

3.  **Configure environment:**
    Create an `.env` file from the example and add your Alchemy WebSocket URL.
    ```bash
    cp .env.example .env
    echo "ALCHEMY_WSS_URL=wss://eth-sepolia.g.alchemy.com/v2/YOUR_API_KEY" >> .env
    ```

4.  **Run the application:**
    ```bash
    go run .
    ```

## 🔧 Configuration

The target address to monitor is configured as a constant in `main.go`:
```go
const targetAddress = "0x7AF963CF6D228E564E2A0AA0DDBF06210B38615D"
```

## 📈 Monitoring & API

The service exposes several HTTP endpoints for observability on port `:8080`.

*   `GET /health`: Health check with uptime and broker status.
*   `GET /metrics`: Prometheus-compatible metrics.
*   `GET /queues`: Real-time statistics for all active queues.
*   `GET /dlq?queue=<name>`: Inspect messages in a queue's Dead Letter Queue.

## 🧪 Testing

The project has a comprehensive test suite with **88.4%** code coverage.

```bash
# Run all tests and generate coverage report
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# Run performance benchmarks for the broker
go test ./broker -bench=. -benchmem

# Run end-to-end system benchmarks
go test . -bench=BenchmarkMessageThroughput -benchtime=3s
```

## 📄 License

This project is licensed under the MIT License. See the `LICENSE` file for details.

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a pull request.

1.  Fork the repository
2.  Create a feature branch
3.  Add tests for new functionality
4.  Ensure all tests pass
5.  Submit a pull request

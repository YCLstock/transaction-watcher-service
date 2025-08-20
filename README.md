# Transaction Watcher Service

This is a simple Go application that connects to the Ethereum Sepolia test network using a WebSocket (WSS) endpoint. It subscribes to new block headers and prints the block number and hash for each new block that is created.

## Prerequisites

- [Go](https://golang.org/) installed on your machine.
- A WebSocket URL from a service like [Alchemy](https://www.alchemy.com/).

## Getting Started

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/YCLstock/transaction-watcher-service.git
    cd transaction-watcher-service
    ```

2.  **Install dependencies:**
    The project uses Go modules. Dependencies will be downloaded automatically when you run the application. You can also run `go mod tidy` to download them manually.

3.  **Configure Your Environment:**
    This project uses a `.env` file to manage secret keys.

    1.  First, create a copy of the example file. In your terminal, run:
        ```bash
        # For Windows
        copy .env.example .env

        # For macOS / Linux
        cp .env.example .env
        ```
    2.  Open the newly created `.env` file in a text editor.
    3.  Add your Alchemy WebSocket URL to the file:
        ```
        ALCHEMY_WSS_URL=wss://eth-sepolia.g.alchemy.com/v2/YOUR_API_KEY
        ```
    > **Important**: The `.env` file is listed in `.gitignore` and should **never** be committed to version control.

4.  **Run the application:**
    ```bash
    go run main.go
    ```

    You should see output indicating a successful connection, and then a new message for each new block that is mined on the Sepolia testnet.

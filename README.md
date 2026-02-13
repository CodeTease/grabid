# Grabid

Grabid is a utility service for grabbing and streaming content from URLs. It consists of a secure Go backend that acts as a proxy/streamer and a modern React frontend for user interaction.

## Features

- **Probe URL**: Check the size and content type of a remote resource without downloading the full content.
- **Stream Content**: Stream data from a remote URL to the client, acting as a proxy.
- **Authentication**: Optional security layer using a secret token (`GRAB_SECRET`) to restrict access.
- **Dockerized**: Fully containerized with Docker Compose for easy deployment.

## Prerequisites

- [Docker](https://www.docker.com/get-started)
- [Docker Compose](https://docs.docker.com/compose/install/)

## Getting Started

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/CodeTease/grabid.git
    cd grabid
    ```

2.  **Start the application:**
    ```bash
    docker-compose up --build
    ```

3.  **Access the application:**
    Open your browser and navigate to `http://localhost`.

## Running with Docker from GHCR

You can also run the application directly from the GitHub Container Registry without cloning the repository.

1.  **Create a Docker network:**
    ```bash
    docker network create grabid-net
    ```

2.  **Start the backend:**
    ```bash
    docker run -d \
      --name grabid-backend \
      --network grabid-net \
      -p 8080:8080 \
      -e GRAB_SECRET=your_secret \
      ghcr.io/codetease/grabid-backend:latest
    ```

3.  **Start the frontend:**
    ```bash
    docker run -d \
      --name grabid-frontend \
      --network grabid-net \
      -p 80:80 \
      ghcr.io/codetease/grabid-frontend:latest
    ```

## Configuration

The application can be configured using environment variables. You can set these in the `docker-compose.yml` file or in a `.env` file.

| Variable | Description | Default |
| :--- | :--- | :--- |
| `PORT` | The port on which the backend service runs. | `8080` |
| `GRAB_SECRET` | Secret token for authentication. If set, requests must include `X-Grab-Token` header. | (Empty/Public) |
| `GRAB_MAX_SIZE` | Maximum file size to stream (e.g. 1GB, 500MB). | `1GB` |
| `GRAB_MAX_CONCURRENT` | Maximum concurrent stream requests. | `5` |
| `GRAB_RATE_LIMIT` | Rate limit per IP (requests-burst). | `1-5` |

### Frontend Development

For local development (`npm run dev`), the frontend can be configured:

| Variable | Description | Default |
| :--- | :--- | :--- |
| `VITE_API_URL` | The URL of the backend API for proxying requests. | `http://localhost:8080` |

## API Endpoints

### 1. Probe URL
HEAD request to get metadata about a remote resource.

- **Endpoint**: `/api/v1/probe`
- **Method**: `HEAD`
- **Query Params**: `url` (The target URL)
- **Headers**: `X-Grab-Token` (If `GRAB_SECRET` is set)
- **Response**:
    - `200 OK`: Returns JSON with `size` and `type`.

### 2. Stream Content
GET request to stream the content of a remote resource.

- **Endpoint**: `/api/v1/stream`
- **Method**: `GET`
- **Query Params**: `url` (The target URL)
- **Headers**: `X-Grab-Token` (If `GRAB_SECRET` is set)
- **Response**: stream of the file content.

## Development

### Backend (Go)

1.  Navigate to the backend directory:
    ```bash
    cd grabid-backend
    ```
2.  Install dependencies:
    ```bash
    go mod download
    ```
3.  Run the server:
    ```bash
    go run main.go
    ```
4.  Run tests:
    ```bash
    go test ./...
    ```

### Frontend (React + Vite)

1.  Navigate to the frontend directory:
    ```bash
    cd grabid-frontend
    ```
2.  Install dependencies:
    ```bash
    npm install
    ```
3.  Start the development server:
    ```bash
    npm run dev
    ```
4.  Lint the code:
    ```bash
    npm run lint
    ```
5.  Build for production:
    ```bash
    npm run build
    ```

## License

[MIT License](LICENSE)

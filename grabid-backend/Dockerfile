# Builder Stage
FROM golang:alpine AS builder

# Install git and ca-certificates (git needed if dependencies were external, ca-certificates needed for final image)
RUN apk update && apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go mod and sum files
COPY go.mod ./
# COPY go.sum ./ # No go.sum because we use only standard library

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

COPY . .

# Build the Go app
# -ldflags="-w -s" reduces binary size by stripping debug info
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o grabid-backend .

# Final Stage
FROM scratch

# Import the user and group files from the builder (optional, for non-root)
# For simplicity, we run as root in scratch or we can copy /etc/passwd

# Copy CA certificates for HTTPS requests
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=builder /app/grabid-backend /grabid-backend

ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/grabid-backend"]

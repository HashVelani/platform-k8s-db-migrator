# Stage 1: build static binary
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /init-container ./cmd/main.go

# Stage 2: scratch image — binary only, nothing else
FROM scratch
COPY --from=builder /init-container /init-container
ENTRYPOINT ["/init-container"]

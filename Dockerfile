# Dockerfile
FROM golang:1.24 AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -o custom-scheduler ./cmd/scheduler

FROM alpine:3.21.3

COPY --from=builder /workspace/custom-scheduler /usr/local/bin/custom-scheduler

EXPOSE 10251

ENTRYPOINT ["/usr/local/bin/custom-scheduler"]

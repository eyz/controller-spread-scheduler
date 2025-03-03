### build

FROM golang:1.24.0-alpine3.21 AS build

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -o controller-spread-scheduler ./cmd/scheduler

### final

# FROM alpine:3.21.3

# Use distroless for smaller and more secure image than Alpine
FROM gcr.io/distroless/static:nonroot AS final

COPY --from=build /workspace/controller-spread-scheduler /usr/local/bin/controller-spread-scheduler

EXPOSE 10251

ENTRYPOINT ["/usr/local/bin/controller-spread-scheduler"]

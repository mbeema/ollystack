FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY . .
RUN go mod tidy && go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o notification-service .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates wget
WORKDIR /app
COPY --from=builder /app/notification-service .
EXPOSE 8080
CMD ["./notification-service"]

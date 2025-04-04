FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY . .
RUN go mod tidy && go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o traffic-generator .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/traffic-generator .
CMD ["./traffic-generator"]

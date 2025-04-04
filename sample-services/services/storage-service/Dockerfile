FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY *.go ./
RUN go mod tidy && go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /storage-service .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /storage-service /storage-service
EXPOSE 8087
CMD ["/storage-service"]

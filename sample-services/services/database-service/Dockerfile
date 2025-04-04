FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY *.go ./
RUN go mod tidy && go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /database-service .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /database-service /database-service
EXPOSE 8086
CMD ["/database-service"]

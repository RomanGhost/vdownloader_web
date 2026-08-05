FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY . .
RUN go build -o webui .

FROM alpine:3.21

WORKDIR /app
COPY --from=builder /app/webui .

CMD ["./webui"]

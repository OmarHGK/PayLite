FROM golang:1.26 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /paylite main.go

FROM alpine:3.20

WORKDIR /app

COPY --from=builder /paylite /app/paylite

EXPOSE 8080

CMD ["/app/paylite"]
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o ctx .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/ctx /usr/local/bin/ctx
EXPOSE 8377
ENTRYPOINT ["ctx", "serve"]

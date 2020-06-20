FROM golang:1.14.1-alpine as builder

WORKDIR /app

COPY . .
 
RUN go mod vendor
RUN go build -o monitor .
 

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/ 
COPY --from=builder /app/monitor .
 
ENTRYPOINT ["./monitor"]
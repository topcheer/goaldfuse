FROM golang:1.17.6-alpine3.15 AS builder
RUN go env -w GO111MODULE=auto \
  && go env -w GOPROXY=https://goproxy.cn,direct  \
  && sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && cat /etc/apk/repositories \
  && apk add --no-cache bash git openssh 
WORKDIR /build
COPY ./ .
RUN go build -o goaldfuse main.go

FROM alpine:latest
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && cat /etc/apk/repositories
WORKDIR /data
RUN apk add --no-cache tzdata curl
COPY --from=builder /build/goaldfuse /usr/bin/goaldfuse
RUN chmod +x /usr/bin/goaldfuse
VOLUME /data
ENTRYPOINT ["/usr/bin/goaldfuse"]

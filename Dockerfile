FROM golang:1.15 as builder

WORKDIR /go/src/github.com/shteou/regcred-injector

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY main.go main.go
COPY handlers handlers
COPY k8s k8s

RUN go build -ldflags="-w -s" main.go
RUN mv main regcred-injector

FROM busybox:glibc as production

COPY --from=builder /go/src/github.com/shteou/regcred-injector/regcred-injector /usr/bin/regcred-injector

CMD ["regcred-injector"]

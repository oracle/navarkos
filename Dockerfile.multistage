FROM gcr.io/google-containers/kube-cross:v1.8.3-3

RUN useradd -u 10001 kube-operator

WORKDIR /go/src/github.com/kubernetes-incubator/navarkos

COPY . .

ARG ldflags

RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/linux/navarkos -installsuffix cgo -ldflags "${ldflags}" ./cmd/...

RUN chmod a+x bin/linux/navarkos

FROM alpine:3.6

COPY --from=0 /etc/passwd /etc/passwd

USER kube-operator

COPY --from=0 /go/src/github.com/kubernetes-incubator/navarkos/bin/linux/navarkos .

CMD ["./navarkos"]
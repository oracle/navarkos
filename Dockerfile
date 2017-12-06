FROM alpine:3.6

COPY bin/linux/navarkos .

CMD ["./navarkos"]
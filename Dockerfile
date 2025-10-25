FROM golang:1.13 as builder
WORKDIR /go/src/github.com/influxdata
COPY influxdb/go.mod influxdb/go.sum ./influxdb/
COPY influxql ./influxql/
WORKDIR /go/src/github.com/influxdata/influxdb
RUN go mod download
COPY influxdb/ ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /usr/bin/influxd ./cmd/influxd
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /usr/bin/influx ./cmd/influx

FROM debian:bullseye-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /usr/bin/influxd /usr/bin/influxd
COPY --from=builder /usr/bin/influx /usr/bin/influx
COPY --from=builder /go/src/github.com/influxdata/influxdb/etc/config.sample.toml /etc/influxdb/influxdb.conf

EXPOSE 8086
VOLUME /var/lib/influxdb

COPY --from=builder /go/src/github.com/influxdata/influxdb/docker/entrypoint.sh /entrypoint.sh
COPY --from=builder /go/src/github.com/influxdata/influxdb/docker/init-influxdb.sh /init-influxdb.sh
ENTRYPOINT ["/entrypoint.sh"]
CMD ["influxd"]

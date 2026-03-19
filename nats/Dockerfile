FROM nats:2.10

COPY conf/nats-server.conf /etc/nats/nats-server.conf
COPY certs /etc/nats/certs

CMD ["-c", "/etc/nats/nats-server.conf"]
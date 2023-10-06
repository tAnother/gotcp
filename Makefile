VHOST=vhost
VROUTER=vrouter

all:
	go build ./cmd/${VHOST}
	go build ./cmd/${VROUTER}

clean:
	rm -f ${VHOST} ${VROUTER}
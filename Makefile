VHOST=vhost
VROUTER=vrouter
OUTPUT_DIR=exec

all: host router

host:
	go build -o ${OUTPUT_DIR}/${VHOST} ./cmd/${VHOST} 

router:
	go build -o ${OUTPUT_DIR}/${VROUTER} ./cmd/${VROUTER} 

clean:
	rm -f ${OUTPUT_DIR}/${VHOST} ${OUTPUT_DIR}/${VROUTER}
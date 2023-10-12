VHOST=vhost
VROUTER=vrouter
OUTPUT_DIR=programs

all:
	go build -o ${OUTPUT_DIR}/${VHOST} ./cmd/${VHOST} 
	go build -o ${OUTPUT_DIR}/${VROUTER} ./cmd/${VROUTER} 

clean:
	rm -f ${OUTPUT_DIR}/${VHOST} ${OUTPUT_DIR}/${VROUTER}
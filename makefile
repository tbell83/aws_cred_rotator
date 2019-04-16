GOLANG = golang:1.12
LDFLAGS := "-s -w"
SDK_PATH = vendor/github.com/aws/aws-sdk-go
WORK_PATH = /go/src/github.com/DramaFever/aws_cred_rotator

.PHONY: all build setup
all: build

setup:
	brew list upx >/dev/null 2>&1 || brew install upx
	git clone https://github.com/aws/aws-sdk-go ${SDK_PATH} || cd ${SDK_PATH} && git pull

build: | setup
	docker run -i --rm -v $(PWD):${WORK_PATH} -w ${WORK_PATH} ${GOLANG} sh -c "GOOS=darwin go build -ldflags ${LDFLAGS}"
	# Packing in the container seems to cause segfaults
	upx --brute aws_cred_rotator
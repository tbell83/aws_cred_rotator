all:
	go build -ldflags="-s -w" && \
	upx --brute aws_cred_rotator
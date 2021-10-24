build:
	go build -ldflags="-s -w" -o exbico-leads-uploader exbico-leads-uploader
build-for-all-platforms:
	./build-for-all-platforms.sh exbico-leads-uploader
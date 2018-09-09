.DEFAULT_GOAL := build

clean: 
	rm -f citium
	
build:
	GOOS=linux GOARCH=amd64 go build -o citium . 

build-tools:
	go build -o citium-cli ./tools

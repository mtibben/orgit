build:
	go build -o orgit .

install:
	go build -o ~/bin/orgit .

release:
	goreleaser release --clean

all: build 

build:
	go build -o secret_inject .

test:
	go test -v ./...

clean:
	rm -f secret_inject 

.PHONY: build run test fmt vet clean

# T7 将扩展为三平台交叉编译；当前为本机开发构建。
build:
	go build -o bin/evergate ./cmd/evergate

run:
	go run ./cmd/evergate

test:
	go test ./...

fmt:
	gofmt -l -w .

vet:
	go vet ./...

clean:
	rm -rf bin

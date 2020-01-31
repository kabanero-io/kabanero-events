IMAGE ?= kabanero/kabanero-events

build:
	docker build -t $(IMAGE) .

push-image:
	docker push $(IMAGE)

local-build:
	go build github.com/kabanero-io/kabanero-events/cmd/kabanero-events/...

lint:
	golint -set_exit_status cmd/... pkg/...

vet:
	go vet github.com/kabanero-io/kabanero-events/...

test:
	go test ./...

format:
	go fmt ./...

local-all: format lint vet test local-build

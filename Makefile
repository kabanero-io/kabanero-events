IMAGE ?= kabanero/kabanero-events

.PHONY: build push-image

build:
	docker build -t $(IMAGE) .

push-image:
	docker push $(IMAGE)

local-build:
	GO111MODULE=off go build ./...

lint:
	golint -set_exit_status

test:
	GO111MODULE=off go test ./...

format:
	GO111MODULE=off go fmt ./...

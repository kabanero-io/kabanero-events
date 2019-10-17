IMAGE ?= kabanero-io/kabanero-webhook

.PHONY: build push-image

build:
	docker build -t $(IMAGE) .

push-image:
	docker push $(IMAGE)

test:
	GO111MODULE=off go test

format:
	go fmt *.go

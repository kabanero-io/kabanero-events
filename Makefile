IMAGE ?= kabanero/kabanero-webhook

.PHONY: build push-image

build:
	docker build -t $(IMAGE) .

push-image:
	docker push $(IMAGE)

test:
ifeq ($(TRAVIS),true)
	go test -v -race ./...
else
	echo "Skipping tests since this is not a Travis build."
endif

format:
	go fmt *.go

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

push-manifest:
	echo "IMAGE="$(IMAGE)
	docker manifest create $(IMAGE) $(IMAGE)-amd64 $(IMAGE)-ppc64le $(IMAGE)-s390x
	docker manifest annotate $(IMAGE) $(IMAGE)-amd64   --os linux --arch amd64
	docker manifest annotate $(IMAGE) $(IMAGE)-ppc64le --os linux --arch ppc64le
	docker manifest annotate $(IMAGE) $(IMAGE)-s390x   --os linux --arch s390x
	docker manifest inspect $(IMAGE)
	docker manifest push $(IMAGE) -p

test:
	go test ./...

format:
	go fmt ./...

local-all: format lint vet test local-build

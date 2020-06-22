# ChubaoFS CSI Makefile

IMAGE_TAG=chubaofs/cfs-csi-driver:3.0.0

.PHONY: all build image push clean

all: build

# dep ensure -vendor-only
build:
	@{ cd build; ./build_cfs_client.sh; }
	@{ cd build; ./build_cfs_node_driver.sh; }

image: build
	@docker build -t $(IMAGE_TAG) ./build
push: image
	@docker push $(IMAGE_TAG)
clean:
	@rm -rf build/bin

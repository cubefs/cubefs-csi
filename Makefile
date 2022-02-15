# CubeFS CSI Makefile

IMAGE_TAG=cubefs/cfs-csi-driver:3.0.0

.PHONY: all build image push clean

all: build

# dep ensure -vendor-only
build:
	@{ cd build; bash ./build_cfs_client.sh; }
	@{ cd build; bash ./build_cfs_csi_driver.sh; }

csi-build:
	@{ cd build; bash ./build_cfs_csi_driver.sh; }

image: build
	@docker build -t $(IMAGE_TAG) ./build
push: image
	@docker push $(IMAGE_TAG)
clean:
	@rm -rf build/bin

# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

REGISTRY_NAME=docker.io/chubaofs
IMAGE_NAME=cfscsi
IMAGE_VERSION=v0.3.0
IMAGE_TAG=$(REGISTRY_NAME)/$(IMAGE_NAME):$(IMAGE_VERSION)
REV=$(shell git describe --long --tags --dirty)

.PHONY: all cfs-build clean cfs-image

all: cfs-build

cfs-build:
	if [ ! -d ./vendor ]; then dep ensure -vendor-only; fi
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o _output/cfsplugin ./app/cfsplugin; \
    mv ./_output/cfsplugin ./pkg/cfs/deploy/
cfs-image: cfs-build
	docker build -t $(IMAGE_TAG) ./pkg/cfs/deploy/.
push: cfs-image
	docker push $(IMAGE_TAG)
clean:
	go clean -r -x
	-rm -rf _output/*

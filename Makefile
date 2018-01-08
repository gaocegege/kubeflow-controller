# Copyright 2018 Caicloud Inc. All Rights Reserved.
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

# Current version of the project.
VERSION ?= v0.0.1

# This repo's root import path (under GOPATH).
ROOT := github.com/caicloud/kubeflow-controller

# Project main package location (can be multiple ones).
CMD_DIR := ./cmd/controller

# Project output directory.
OUTPUT_DIR := ./bin

# Git commit sha.
GitSHA := $(shell git rev-parse --short HEAD)

# Golang standard bin directory.
BIN_DIR := $(GOPATH)/bin

# Golang packages except vendor.
PACKAGES := $(shell go list ./... | grep -v /vendor/ )

build:
	go build -i -v -o $(OUTPUT_DIR)/kubeflow-controller \
	  -ldflags "-s -w -X $(ROOT)/version.Version=$(VERSION) \
	            -X $(ROOT)/version.GitSHA=$(GitSHA)" \
	  $(CMD_DIR) \

test:
	go test $(PACKAGES)

clean:
	-rm -vrf ${OUTPUT_DIR}

.PHONY: clean build

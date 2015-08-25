MAKEFLAGS += --warn-undefined-variables
SHELL := bash
.SHELLFLAGS := -e -c

RM := rm -rf

BUILD_DATE := $(shell date +%Y-%m-%d" "%H:%M:%S" "%z)
GIT_HASH := $(shell git rev-parse --short HEAD)
GIT_DATE := $(shell git show -s --format=%ci HEAD)
GIT_DIRTY := $(shell [[ $$(git diff --shortstat 2> /dev/null | tail -n1) != "" ]] && echo "+")



all: build

build:
	set -e
	go build -ldflags "-X main.gitHashString=${GIT_HASH}${GIT_DIRTY} -X \"main.buildDateString=${BUILD_DATE}\" -X \"main.gitDateString=${GIT_DATE}\"" gym.go

clean:
	rm gym

# MAINTAINER: Gorka Lerchundi Osa <glertxundi@gmail.com>
.PHONY: build fmt generate lint test vet clean

PACKAGE = github.com/glerchundi/loadtesting-ws
NAME    = `echo $(PACKAGE) | rev | cut -d/ -f1 | rev`
GIT_REV = `git rev-parse --verify HEAD`
PKGS    = `go list ./... | grep -v /vendor/`
OS      = linux windows darwin
APP     = listener benchmarker

build:
ifndef VERSION
	$(error VERSION is not set)
else
	@for app in $(APP) ; do \
		for os in $(OS) ; do \
			ext=""; \
			if [ "$$os" == "windows" ]; then \
				ext=".exe"; \
			fi; \
			GO15VENDOREXPERIMENT=1 \
			GOOS=$$os GOARCH=amd64 CGO_ENABLED=0  \
			go build \
				-a -x -tags netgo -installsuffix cgo -installsuffix netgo \
				-ldflags " \
					-X main.Version=$(VERSION) \
					-X main.GitRev=$(GIT_REV) \
				" \
				-o ./bin/$$app-$(VERSION)-$$os-amd64$$ext \
				./cmd/$$app; \
		done; \
	done
endif

fmt:
	@echo "Running gofmt..."
	@for dir in $(PKGS) ; do \
		res=$$(GO15VENDOREXPERIMENT=1 gofmt -l $(GOPATH)/src/$$dir/.); \
		if [ -n "$$res" ]; then \
			echo "gofmt checking failed:\n$$res"; \
			exit 255; \
		fi \
	done

generate:
	@echo "Running go generate..."
	GO15VENDOREXPERIMENT=1 go generate $(PKGS)

lint:
	@echo "Running golint..."
	for dir in $(PKGS) ; do GO15VENDOREXPERIMENT=1 golint $$dir; done

test:
	@echo "Running go test..."
	GO15VENDOREXPERIMENT=1 go test $(PKGS)

vet:
	@echo "Running go vet..."
	GO15VENDOREXPERIMENT=1 go vet $(PKGS)

clean:
	rm -f ./bin/*

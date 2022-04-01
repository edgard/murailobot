ACCOUNT := ghcr.io/edgard
PROJECT := murailobot
IMAGE := $(ACCOUNT)/$(PROJECT)

build:
	$(info Make: Building image.)
	@go mod tidy
	@go mod download
	@go build ./...
	@docker build -t $(IMAGE) -f Dockerfile .

start:
	$(info Make: Starting container.)
	@docker run -dit --name $(PROJECT) $(IMAGE):latest

stop:
	$(info Make: Stopping container.)
	@docker stop $(PROJECT)
	@docker rm $(PROJECT)

restart:
	$(info Make: Restarting container.)
	@make -s stop
	@make -s start

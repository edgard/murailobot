ACCOUNT := docker.home.edgard.org
SERVICE := murailobot
IMAGE := $(ACCOUNT)/$(SERVICE)

build:
	$(info Make: Building image.)
	@docker build -t $(IMAGE) -f Dockerfile .

start:
	$(info Make: Starting container.)
	@docker run -dit --name $(SERVICE) $(IMAGE):latest

stop:
	$(info Make: Stopping container.)
	@docker stop $(SERVICE)
	@docker rm $(SERVICE)

restart:
	$(info Make: Restarting container.)
	@make -s stop
	@make -s start

push:
	$(info Make: Pushing image.)
	@docker push $(IMAGE):latest

pull:
	$(info Make: Pulling image.)
	@docker pull $(IMAGE):latest

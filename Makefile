DOCKER_COMPOSE ?= docker compose
GO_IMAGE ?= golang:1.26.10-alpine3.23
APP_NETWORK ?= go-recorder_app-network

.PHONY: serve api record-worker media-worker product-worker migrate doctor up restart migrate-up debug-api debug-record-worker debug-both debug-stop docker-build docker-up docker-down docker-restart

serve:
	go run ./cmd/main serve

api:
	go run ./cmd/api

record-worker:
	go run ./cmd/record-worker

media-worker:
	go run ./cmd/media-worker

product-worker:
	go run ./cmd/product-worker

migrate:
	$(MAKE) migrate-up

doctor:
	$(DOCKER_COMPOSE) ps
	$(DOCKER_COMPOSE) exec -T rabbitmq rabbitmq-diagnostics -q check_port_connectivity
	$(DOCKER_COMPOSE) exec -T api wget -qO- http://127.0.0.1:8085/health
	$(DOCKER_COMPOSE) exec -T record-worker wget -qO- http://127.0.0.1:8090/health

up:
	$(DOCKER_COMPOSE) build
	python3 -I scripts/install/retire-legacy-services.py --check-migration --compose $(DOCKER_COMPOSE)
	$(DOCKER_COMPOSE) up -d --no-build

restart:
	python3 -I scripts/install/retire-legacy-services.py --check-migration --compose $(DOCKER_COMPOSE)
	$(DOCKER_COMPOSE) down
	$(MAKE) up

migrate-up:
	python3 -I scripts/install/retire-legacy-services.py --check-migration --compose $(DOCKER_COMPOSE)
	docker run --rm \
		--network $(APP_NETWORK) \
		--env-file .env \
		-v "$(CURDIR):/src" \
		-w /src \
		$(GO_IMAGE) go run ./cmd/main migrate

debug-api:
	python3 -I scripts/install/retire-legacy-services.py --check-migration --compose $(DOCKER_COMPOSE)
	$(DOCKER_COMPOSE) stop api
	$(DOCKER_COMPOSE) --profile debug up -d --build api-debug

debug-record-worker:
	python3 -I scripts/install/retire-legacy-services.py --check-migration --compose $(DOCKER_COMPOSE)
	$(DOCKER_COMPOSE) stop record-worker
	$(DOCKER_COMPOSE) --profile debug up -d --build record-worker-debug

debug-both:
	python3 -I scripts/install/retire-legacy-services.py --check-migration --compose $(DOCKER_COMPOSE)
	$(DOCKER_COMPOSE) stop api record-worker
	$(DOCKER_COMPOSE) --profile debug up -d --build api-debug record-worker-debug

debug-stop:
	$(DOCKER_COMPOSE) stop api-debug record-worker-debug

docker-build:
	$(DOCKER_COMPOSE) build

docker-up:
	$(MAKE) up

docker-down:
	$(DOCKER_COMPOSE) down

docker-restart:
	$(MAKE) restart

# RELEASE_ARGS содержит явные параметры --environment, --env-file, --manifest и --project.
.PHONY: release-validate release-pull release-migrate release-drain release-resume release-deploy release-rollback release-build release-package
release-validate:
	bash scripts/release/release.sh validate $(RELEASE_ARGS)
release-pull:
	bash scripts/release/release.sh pull $(RELEASE_ARGS)
release-migrate:
	bash scripts/release/release.sh migrate $(RELEASE_ARGS)
release-drain:
	bash scripts/release/release.sh drain $(RELEASE_ARGS)
release-resume:
	bash scripts/release/release.sh resume $(RELEASE_ARGS)
release-deploy:
	bash scripts/release/release.sh deploy $(RELEASE_ARGS)
release-rollback:
	bash scripts/release/release.sh rollback $(RELEASE_ARGS)
release-build:
	bash scripts/release/build.sh $(BUILD_ARGS)
release-package:
	bash scripts/release/package.sh $(PACKAGE_ARGS)

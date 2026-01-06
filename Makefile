# Устанавливаем цель по умолчанию. При вызове 'make' без аргументов будет показана справка.
.DEFAULT_GOAL := help

# Выносим повторяющиеся команды и имена в переменные для гибкости и уменьшения дублирования.
COMPOSE_FILE := docker-compose.yml
COMPOSE      := docker compose -f $(COMPOSE_FILE)

# Имена сервисов из docker-compose
WEBSOCKERT1_SERVICE := webs1
WEBSOCKERT2_SERVICE := webs2

# Имена контейнеров
WEBSOCKERT1_CONTAINER := websock1
WEBSOCKERT2_CONTAINER := websock2

.PHONY: help up start down clean rebuild ps logs1 logs2 logs shell1 shell2

# ---- HELP ----
help: ## показать цели
	@echo "Usage: make [target]"
	@echo ""
	@grep -E '^[a-zA-Z0-9_-]+:.*##' Makefile | awk 'BEGIN {FS=":"}; {printf "\033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ---- DOCKER ----
up: ## собрать и запустить всё окружение
	$(COMPOSE) up -d --build --force-recreate
	@echo "\n🚀 App stack started"
	@echo "   - Websocket sever 1: https://localhost:8443"
	@echo "   - Websocket sever 2: https://localhost:8444"

start: ## запустить окружение без пересборки
	$(COMPOSE) start
	@echo "\n▶️ Services started"

down: ## остановить и удалить все контейнеры
	$(COMPOSE) down --remove-orphans -v
	@echo "\n🧹 All containers stopped and cleaned"

clean: ## очистить систему Docker
	$(COMPOSE) down --rmi local --remove-orphans
	@echo "\n🧽 Docker system cleaned."

rebuild: ## полная пересборка проекта
	$(MAKE) down
	$(MAKE) up
	@echo "\n♻️  Full environment rebuilt and started."

ps: ## список и статусы контейнеров
	$(COMPOSE) ps

logs1: ## логи первого вебсокета
	$(COMPOSE) logs -f $(WEBSOCKERT1_SERVICE)

logs2: ## логи второго вебсокета
	$(COMPOSE) logs -f $(WEBSOCKERT2_SERVICE)

logs: ## логи всех контейнеров
	$(COMPOSE) logs -f

shell1: ## shell в контейнер websock1
	docker exec -it $(WEBSOCKERT1_CONTAINER) /bin/sh

shell2: ## shell в контейнер websock2
	docker exec -it $(WEBSOCKERT2_CONTAINER) /bin/sh

# ---- GO LOCAL ----
vet: ## запустить go vet
	go vet ./...

fmt: ## форматировать код
	go fmt ./...

run: ## запустить локально без Docker
	go run cmd/server/main.go
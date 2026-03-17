LOCAL_DOCKER_COMPOSE_FILE=.local/docker-compose.yml
DB_DRIVER=postgres
DB_STRING=postgres://user:password@localhost:5432/metric-sherlock?sslmode=disable
MIGRATIONS_DIR=migrations/postgres

compose-up:
	docker compose -f $(LOCAL_DOCKER_COMPOSE_FILE) up -d

compose-down:
	docker compose -f $(LOCAL_DOCKER_COMPOSE_FILE) down

goose-up:
	goose -dir $(MIGRATIONS_DIR) $(DB_DRIVER) "$(DB_STRING)" up

goose-down:
	goose -dir $(MIGRATIONS_DIR) $(DB_DRIVER) "$(DB_STRING)" down

run:
	go run ./cmd
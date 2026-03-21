LOCAL_DOCKER_COMPOSE_FILE=.local/docker-compose.yml
DB_DRIVER=postgres
DB_STRING=postgres://user:password@localhost:5432/metric-sherlock?sslmode=disable
MIGRATIONS_DIR=migrations/postgres


.PHONY: compose-up
compose-up:
	docker compose -f $(LOCAL_DOCKER_COMPOSE_FILE) up -d

.PHONY: compose-down
compose-down:
	docker compose -f $(LOCAL_DOCKER_COMPOSE_FILE) down

.PHONY:  goose-up
goose-up:
	goose -dir $(MIGRATIONS_DIR) $(DB_DRIVER) "$(DB_STRING)" up

.PHONY: goose-down
goose-down:
	goose -dir $(MIGRATIONS_DIR) $(DB_DRIVER) "$(DB_STRING)" down

.PHONY: run
run:
	go run ./cmd


PROTO_FILE=proto/metricsherlock/targetgroups/v1/target_groups.proto
GOOGLEAPIS_DIR=$(shell go env GOPATH)/pkg/mod/github.com/grpc-ecosystem/grpc-gateway@v1.16.0/third_party/googleapis

.PHONY: proto
proto:
	mkdir -p internal/httpapi/static
	protoc -I . -I $(GOOGLEAPIS_DIR) \
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=. --grpc-gateway_opt=paths=source_relative \
		--openapiv2_out=internal/httpapi/static \
		--openapiv2_opt=allow_merge=true,merge_file_name=target_groups \
		$(PROTO_FILE)

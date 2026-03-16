.PHONY: up down api scraper migrate mobile

# Start all services
up:
	docker compose up -d

# Stop all services
down:
	docker compose down

# Run API locally (requires Go)
api:
	cd services/api && go run main.go

# Run scraper locally (requires Python)
scraper:
	cd services/scraper && python -m src.main

# Run mobile app (requires Node/Expo)
mobile:
	cd apps/mobile && npx expo start

# Run DB migrations (via psql)
migrate:
	docker compose exec postgres psql -U postgres -d swissgrocery -f /docker-entrypoint-initdb.d/001_initial.sql

# Fresh start — wipe DB and rebuild
reset:
	docker compose down -v
	docker compose up -d

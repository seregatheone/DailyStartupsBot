.PHONY: test test-backend test-bot run-backend dry-run-backend run-bot

test: test-backend test-bot

test-backend:
	cd backend && go test ./...

test-bot:
	cd bot && python3 -m unittest discover -s tests

run-backend:
	cd backend && DAILY_STARTUPS_DRY_RUN=false go run ./cmd/backend

dry-run-backend:
	cd backend && DAILY_STARTUPS_DRY_RUN=true go run ./cmd/backend

run-bot:
	cd bot && python3 -m daily_startups_bot

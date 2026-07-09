.PHONY: test test-backend test-bot run-backend run-bot

test: test-backend test-bot

test-backend:
	cd backend && go test ./...

test-bot:
	cd bot && python3 -m unittest discover -s tests

run-backend:
	cd backend && go run ./cmd/backend

run-bot:
	cd bot && python3 -m daily_startups_bot

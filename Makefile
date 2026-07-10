.PHONY: test test-backend test-bot check-localization apply-telegram-metadata run-backend dry-run-backend run-bot

test: test-backend test-bot check-localization

test-backend:
	cd backend && go test ./...

test-bot:
	cd bot && python3 -m unittest discover -s tests

check-localization:
	cd bot && python3 -m unittest discover -s tests -p 'test_localization.py'
	cd bot && python3 -m daily_startups_bot.metadata --check

apply-telegram-metadata:
	cd bot && python3 -m daily_startups_bot.metadata --apply

run-backend:
	cd backend && DAILY_STARTUPS_DRY_RUN=false go run ./cmd/backend

dry-run-backend:
	cd backend && DAILY_STARTUPS_DRY_RUN=true go run ./cmd/backend

run-bot:
	cd bot && python3 -m daily_startups_bot

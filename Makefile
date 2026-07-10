.PHONY: test test-backend test-bot test-ops check-localization apply-telegram-metadata run-backend dry-run-backend run-bot live-up live-smoke telegram-e2e telegram-e2e-checklist

test: test-backend test-bot test-ops check-localization

test-backend:
	cd backend && go test ./...

test-bot:
	cd bot && python3 -m unittest discover -s tests

test-ops:
	python3 -m unittest discover -s scripts/tests

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

live-up:
	python3 scripts/live.py run

live-smoke:
	python3 scripts/live.py smoke

telegram-e2e:
	python3 scripts/telegram_e2e.py run

telegram-e2e-checklist:
	python3 scripts/telegram_e2e.py checklist

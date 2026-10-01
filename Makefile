.PHONY: web server test itest check hooks dev up down

web:            ## Build the web app and copy it into the Go embed folder
	cd web && npm ci && npm run build
	rm -rf server/internal/web/dist && cp -r web/dist server/internal/web/dist

server: web     ## Build the roosty binary
	cd server && CGO_ENABLED=0 go build -o ../roosty ./cmd/roosty

hooks:          ## Install the git hooks that block sensitive commits
	git config core.hooksPath scripts/hooks

check:          ## Check every tracked file for secrets and private data
	scripts/check-repo.sh --all

test:           ## Run the Go tests
	cd server && go test ./...

itest:          ## Integration tests against a real IMAP/SMTP server in Docker
	docker compose -p roosty-itest -f deploy/test/compose.yml up -d --wait
	cd server && ROOSTY_IT_IMAP=127.0.0.1:13143 ROOSTY_IT_SMTP=127.0.0.1:13025 go test -tags integration -count=1 ./internal/server/ ; \
	  status=$$?; cd .. && docker compose -p roosty-itest -f deploy/test/compose.yml down; exit $$status

up:             ## Start the local test stack (Roosty + GreenMail + sample emails)
	docker compose up --build -d && docker compose logs -f roosty

down:           ## Stop the local stack and delete its data
	docker compose down -v

.PHONY: web server test itest e2e check hooks dev up down

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

itest:          ## Integration tests against GreenMail and Dovecot in Docker
	docker compose -p roosty-itest -f deploy/test/compose.yml up -d
	@sleep 4
	cd server && ROOSTY_IT_IMAP=127.0.0.1:13143 ROOSTY_IT_SMTP=127.0.0.1:13025 go test -tags integration -count=1 ./internal/server/ ; s1=$$?; \
	  ROOSTY_IT_IMAP=127.0.0.1:13144 ROOSTY_IT_IMAP_SECURITY=starttls ROOSTY_IT_SKIP_VERIFY=1 ROOSTY_IT_SMTP=none go test -tags integration -count=1 ./internal/server/ ; s2=$$?; \
	  cd .. && docker compose -p roosty-itest -f deploy/test/compose.yml down; exit $$((s1 + s2))

e2e:            ## Browser walkthrough against a fresh Docker stack
	docker compose down -v && docker compose up --build -d
	docker compose wait seed
	until curl -fs http://localhost:8080/healthz >/dev/null; do sleep 1; done
	cd e2e && npm ci --no-audit --no-fund && npm test

up:             ## Start the local test stack (Roosty + GreenMail + sample emails)
	docker compose up --build -d && docker compose logs -f roosty

down:           ## Stop the local stack and delete its data
	docker compose down -v

.PHONY: build run clean test test-race test-coverage

build:
	@echo "Building frontend..."
	cd frontend && NEXT_EXPORT=1 pnpm build
	@echo "Copying frontend to backend..."
	@rm -rf backend/frontend
	cp -r frontend/out backend/frontend
	@echo "Building backend..."
	cd backend && go build -o gobot

run: backend/gobot
	cd backend && ./gobot

clean:
	rm -rf backend/frontend backend/gobot
	cd frontend && rm -rf out .next

# Go tests (delegate to backend Makefile)
test:
	$(MAKE) -C backend test

test-race:
	$(MAKE) -C backend test-race

test-coverage:
	$(MAKE) -C backend test-coverage

backend/gobot:
	$(MAKE) build

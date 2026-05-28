.PHONY: test lint dev server-test mobile-test

test:
	./scripts/test.sh

lint:
	./scripts/lint.sh

dev:
	./scripts/dev.sh

server-test:
	cd server && go test ./...

mobile-test:
	cd mobile && flutter test


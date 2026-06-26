.PHONY: dev build clean web-build go-build

BINARY := cost-board

dev: web-deps
	@echo "Starting Go backend on :8083 and Vite dev server on :5173..."
	@echo "Open http://localhost:5173"
	cd web && npx vite --proxy /api:http://127.0.0.1:8083 &
	go run . --port 8083

web-deps:
	cd web && npm install

web-build: web-deps
	cd web && npm run build

go-build: web-build
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY) .

build: go-build

clean:
	rm -rf web/dist $(BINARY)
	rm -rf data

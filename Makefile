.PHONY: all build frontend clean test help

# Default target
all: build

# Build the entire application (frontend + backend)
build: frontend
	@echo "Building Go application..."
	go build -o gotohp

# Build CLI-only version
build-cli: frontend
	@echo "Building CLI-only version..."
	go build -tags cli -o gotohp-cli

# Build frontend assets
frontend:
	@echo "Installing frontend dependencies..."
	cd frontend && npm install
	@echo "Building frontend..."
	cd frontend && npm run build:dev

# Build frontend for production
frontend-prod:
	@echo "Installing frontend dependencies..."
	cd frontend && npm install
	@echo "Building frontend for production..."
	cd frontend && npm run build

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f gotohp gotohp-cli
	rm -rf frontend/dist/*
	@mkdir -p frontend/dist && touch frontend/dist/.gitkeep

# Run tests
test:
	go test ./...

# Show help
help:
	@echo "Available targets:"
	@echo "  make build        - Build the application (default)"
	@echo "  make build-cli    - Build CLI-only version"
	@echo "  make frontend     - Build frontend assets only"
	@echo "  make frontend-prod - Build frontend for production"
	@echo "  make clean        - Remove build artifacts"
	@echo "  make test         - Run tests"
	@echo "  make help         - Show this help message"

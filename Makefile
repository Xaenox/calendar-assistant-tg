.PHONY: build run clean docker docker-run setup test

# Build the application
build:
	go build -o calendar-assistant ./cmd/bot

# Run the application
run: build
	./calendar-assistant

# Clean build artifacts
clean:
	rm -f calendar-assistant
	rm -rf tmp/*

# Build Docker image
docker:
	docker build -t calendar-assistant .

# Run with Docker
docker-run: docker
	docker run --env-file .env -v ./tmp:/app/tmp calendar-assistant

# Run with Docker Compose
docker-compose:
	docker-compose up -d

# Setup the project
setup:
	@if [ -f setup.sh ]; then \
		chmod +x setup.sh && ./setup.sh; \
	elif [ -f setup.bat ]; then \
		setup.bat; \
	else \
		echo "No setup script found"; \
	fi

# Run tests
test:
	go test -v ./... 
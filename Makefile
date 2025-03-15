.PHONY: build run clean

# Build the application
build:
	go build -o calendar-assistant ./cmd/bot

# Run the application
run: build
	./calendar-assistant

# Clean build artifacts
clean:
	rm -f calendar-assistant 
#!/bin/bash

# Check if .env file exists
if [ -f .env ]; then
    echo ".env file already exists. Skipping creation."
else
    # Create .env file
    echo "Creating .env file..."
    echo "TELEGRAM_BOT_TOKEN=your_telegram_bot_token" > .env
    echo "OPENAI_API_KEY=your_openai_api_key" >> .env
    echo ".env file created. Please update it with your actual tokens."
fi

# Create tmp directory if it doesn't exist
if [ ! -d "tmp" ]; then
    echo "Creating tmp directory..."
    mkdir -p tmp
fi

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "Go is not installed. Please install Go before continuing."
    exit 1
fi

# Install dependencies
echo "Installing dependencies..."
go mod download

echo "Setup complete! You can now build and run the application with 'make run' or use Docker with 'docker-compose up -d'." 
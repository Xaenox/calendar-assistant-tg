#!/bin/bash

# Check if .env file exists
if [ -f .env ]; then
    echo ".env file already exists. Skipping creation."
else
    # Create .env file
    echo "Creating .env file..."
    echo "# Telegram Bot Token (get from @BotFather)" > .env
    echo "TELEGRAM_BOT_TOKEN=your_telegram_bot_token" >> .env
    echo "" >> .env
    echo "# OpenAI API Key" >> .env
    echo "OPENAI_API_KEY=your_openai_api_key" >> .env
    echo "" >> .env
    echo "# Optional: Set this to use a specific assistant ID" >> .env
    echo "# If empty, the app will create a new assistant or use an existing one with the name \"Calendar Assistant\"" >> .env
    echo "OPENAI_ASSISTANT_ID=" >> .env
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

echo "Setup complete!"
echo ""
echo "To run the bot locally:"
echo "1. Update the .env file with your actual tokens"
echo "2. Build and run with: make run"
echo ""
echo "To run with Docker:"
echo "1. Update the .env file with your actual tokens"
echo "2. Build and run with: docker-compose up -d"
echo ""
echo "For iPhone users: Use this shortcut to easily add calendar events:"
echo "https://www.icloud.com/shortcuts/db9d3a471c414a1abd2ba7b960395bee" 
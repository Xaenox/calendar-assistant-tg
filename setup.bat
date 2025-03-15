@echo off
echo Calendar Assistant Setup

REM Check if .env file exists
if exist .env (
    echo .env file already exists. Skipping creation.
) else (
    echo Creating .env file...
    echo TELEGRAM_BOT_TOKEN=your_telegram_bot_token > .env
    echo OPENAI_API_KEY=your_openai_api_key >> .env
    echo .env file created. Please update it with your actual tokens.
)

REM Create tmp directory if it doesn't exist
if not exist tmp (
    echo Creating tmp directory...
    mkdir tmp
)

REM Check if Go is installed
where go >nul 2>nul
if %ERRORLEVEL% neq 0 (
    echo Go is not installed. Please install Go before continuing.
    exit /b 1
)

REM Install dependencies
echo Installing dependencies...
go mod download

echo Setup complete! You can now build and run the application with 'go build -o calendar-assistant ./cmd/bot' and './calendar-assistant'
echo Or use Docker with 'docker-compose up -d'

pause 
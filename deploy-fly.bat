@echo off
setlocal enabledelayedexpansion
echo Calendar Assistant - Fly.io Deployment

REM Check if flyctl is installed
where flyctl >nul 2>nul
if %ERRORLEVEL% neq 0 (
    echo flyctl is not installed. Please install it first:
    echo https://fly.io/docs/hands-on/install-flyctl/
    exit /b 1
)

REM Check if logged in to Fly.io
flyctl auth whoami >nul 2>nul
if %ERRORLEVEL% neq 0 (
    echo You are not logged in to Fly.io. Please login first:
    echo flyctl auth login
    exit /b 1
)

REM Check if .env file exists
if not exist .env (
    echo Error: .env file not found. Please create it first.
    exit /b 1
)

REM Create the volume if it doesn't exist
flyctl volumes list | findstr "calendar_assistant_data" >nul
if %ERRORLEVEL% neq 0 (
    echo Creating persistent volume...
    flyctl volumes create calendar_assistant_data --size 1 --region iad
)

REM Set secrets from .env file
echo Setting up secrets from .env file...
for /f "tokens=*" %%a in (.env) do (
    set line=%%a
    if not "!line:~0,1!"=="#" (
        if not "!line!"=="" (
            echo Setting secret: !line!
            flyctl secrets set "!line!" --app calendar-assistant
        )
    )
)

REM Deploy the application
echo Deploying application to Fly.io...
flyctl deploy

echo Deployment complete!
echo Your bot should now be running on Fly.io.
echo You can check the status with: flyctl status
echo And view logs with: flyctl logs

pause 
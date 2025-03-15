#!/bin/bash

# Exit on error
set -e

# Check if flyctl is installed
if ! command -v flyctl &> /dev/null; then
    echo "flyctl is not installed. Please install it first:"
    echo "curl -L https://fly.io/install.sh | sh"
    exit 1
fi

# Check if logged in to Fly.io
if ! flyctl auth whoami &> /dev/null; then
    echo "You are not logged in to Fly.io. Please login first:"
    echo "flyctl auth login"
    exit 1
fi

# Check if .env file exists
if [ ! -f .env ]; then
    echo "Error: .env file not found. Please create it first."
    exit 1
fi

# Create the volume if it doesn't exist
if ! flyctl volumes list | grep -q "calendar_assistant_data"; then
    echo "Creating persistent volume..."
    flyctl volumes create calendar_assistant_data --size 1 --region iad
fi

# Set secrets from .env file
echo "Setting up secrets from .env file..."
while IFS= read -r line || [[ -n "$line" ]]; do
    # Skip comments and empty lines
    if [[ $line =~ ^#.*$ ]] || [[ -z $line ]]; then
        continue
    fi
    
    # Extract key and value
    key=$(echo "$line" | cut -d '=' -f 1)
    value=$(echo "$line" | cut -d '=' -f 2-)
    
    # Set secret
    echo "Setting secret: $key"
    flyctl secrets set "$key=$value" --app calendar-assistant
done < .env

# Deploy the application
echo "Deploying application to Fly.io..."
flyctl deploy

echo "Deployment complete!"
echo "Your bot should now be running on Fly.io."
echo "You can check the status with: flyctl status"
echo "And view logs with: flyctl logs" 
# Calendar Assistant Telegram Bot

A Telegram bot that extracts event information from text or images and creates calendar files (.ics) that can be imported into any calendar application.

## Features

- Extract event details from text descriptions
- Extract event details from images (screenshots, photos of event announcements)
- Timezone support with both IANA names and GMT offsets
- All-day event detection
- Customizable user preferences
- Easy calendar import

## Setup

### Prerequisites

- Go 1.18 or higher
- Telegram Bot Token (from [@BotFather](https://t.me/BotFather))
- OpenAI API Key

### Installation

1. Clone the repository:
   ```
   git clone https://github.com/yourusername/calendar-assistant.git
   cd calendar-assistant
   ```

2. Create a `.env` file in the project root with the following variables:
   ```
   TELEGRAM_BOT_TOKEN=your_telegram_bot_token
   OPENAI_API_KEY=your_openai_api_key
   OPENAI_ASSISTANT_ID=optional_assistant_id
   ```

3. Build the application:
   ```
   go build -o calendar-assistant ./cmd/bot
   ```

4. Run the bot:
   ```
   ./calendar-assistant
   ```

## Usage

1. Start a chat with your bot on Telegram
2. Set your timezone using the `/timezone` command
3. Send a text description of an event or an image containing event details
4. The bot will extract the event information and send you an .ics file
5. Import the .ics file into your calendar application

### Commands

- `/start` - Start the bot
- `/help` - Show help information
- `/timezone` - View or set your timezone (e.g., `/timezone Europe/London` or `/timezone GMT+3`)
- `/clear` - Clear your conversation history

### iPhone Users

For easier setup on iPhone, use this shortcut to automatically add .ics files to your calendar:
[Calendar Import Shortcut](https://www.icloud.com/shortcuts/db9d3a471c414a1abd2ba7b960395bee)

## How It Works

1. The bot receives a message containing text or an image
2. It uses OpenAI's API to extract event information
3. The event details are converted to an .ics file with the user's timezone
4. The .ics file is sent back to the user for import into their calendar

## License

[MIT License](LICENSE) 
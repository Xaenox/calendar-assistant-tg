package telegram

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"calendar-assistant/pkg/calendar"
	"calendar-assistant/pkg/openai"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// UserPreferences stores user-specific settings
type UserPreferences struct {
	Timezone string // IANA timezone name (e.g., "Europe/London", "America/New_York")
}

// Bot represents a Telegram bot
type Bot struct {
	bot             *tgbotapi.BotAPI
	openaiClient    *openai.Client
	userPreferences map[string]*UserPreferences // Map of userID -> preferences
	prefMutex       sync.RWMutex                // Mutex to protect the preferences map
}

// NewBot creates a new Telegram bot
func NewBot(token string, openaiClient *openai.Client) (*Bot, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create Telegram bot: %w", err)
	}

	// Enable debugging
	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)

	b := &Bot{
		bot:             bot,
		openaiClient:    openaiClient,
		userPreferences: make(map[string]*UserPreferences),
	}

	// Set up command autocompletions
	if err := b.setupCommands(); err != nil {
		log.Printf("Warning: Failed to set up command autocompletions: %v", err)
	}

	return b, nil
}

// setupCommands sets up command autocompletions for the bot
func (b *Bot) setupCommands() error {
	// Regular user commands
	commands := []tgbotapi.BotCommand{
		{
			Command:     "start",
			Description: "Start the bot",
		},
		{
			Command:     "help",
			Description: "Show help information",
		},
		{
			Command:     "timezone",
			Description: "View or set your timezone (e.g., /timezone Europe/London or /timezone GMT+3)",
		},
		{
			Command:     "clear",
			Description: "Clear your conversation history",
		},
		{
			Command:     "refresh_commands",
			Description: "Admin only: Refresh the bot's command list",
		},
	}

	// Set regular commands for all users
	config := tgbotapi.NewSetMyCommands(commands...)
	_, err := b.bot.Request(config)
	if err != nil {
		return fmt.Errorf("failed to set regular commands: %w", err)
	}

	// TODO: In the future, implement admin-specific commands using scopes
	// Example:
	// adminCommands := append(commands, tgbotapi.BotCommand{Command: "refresh_commands", Description: "Refresh the bot's command list"})
	// adminScope := tgbotapi.NewBotCommandScopeChat(adminChatID)
	// adminConfig := tgbotapi.NewSetMyCommandsWithScope(adminScope, adminCommands...)

	log.Println("Successfully set up command autocompletions")
	return nil
}

// getUserPreferences gets or creates user preferences
func (b *Bot) getUserPreferences(userID string) *UserPreferences {
	b.prefMutex.RLock()
	prefs, exists := b.userPreferences[userID]
	b.prefMutex.RUnlock()

	if !exists {
		// Create default preferences
		prefs = &UserPreferences{
			Timezone: "UTC", // Default to UTC
		}
		b.prefMutex.Lock()
		b.userPreferences[userID] = prefs
		b.prefMutex.Unlock()
	}

	return prefs
}

// setUserTimezone sets the timezone for a user
func (b *Bot) setUserTimezone(userID string, timezone string) {
	prefs := b.getUserPreferences(userID)

	b.prefMutex.Lock()
	prefs.Timezone = timezone
	b.prefMutex.Unlock()

	log.Printf("Set timezone for user %s to %s", userID, timezone)
}

// Start starts the bot
func (b *Bot) Start() error {
	log.Println("Setting up update configuration...")
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	log.Println("Getting updates channel...")
	updates := b.bot.GetUpdatesChan(u)
	log.Println("Update channel established, waiting for messages...")

	for update := range updates {
		log.Printf("Received update: %+v", update)
		if update.Message == nil {
			log.Println("Update contains no message, skipping")
			continue
		}

		log.Printf("Processing message: %s from user: %s", update.Message.Text, update.Message.From.UserName)
		go b.handleMessage(update.Message)
	}

	return nil
}

// handleMessage handles a message from a user
func (b *Bot) handleMessage(message *tgbotapi.Message) {
	ctx := context.Background()
	chatID := message.Chat.ID
	userID := fmt.Sprintf("%d", message.From.ID) // Use the Telegram user ID as the unique identifier
	messageID := message.MessageID               // Store the original message ID for replies
	log.Printf("Handling message in chat ID: %d from user ID: %s, message ID: %d", chatID, userID, messageID)

	// Handle commands
	if message.IsCommand() {
		log.Printf("Received command: %s", message.Command())
		switch message.Command() {
		case "start":
			welcomeText := "Welcome to Calendar Assistant! I can help you create calendar events from text or images.\n\n📱 iPhone users: For easier setup, use this shortcut to automatically add .ics files to your calendar:\nhttps://www.icloud.com/shortcuts/db9d3a471c414a1abd2ba7b960395bee"
			msg := tgbotapi.NewMessage(chatID, welcomeText)
			msg.ReplyToMessageID = messageID
			if _, err := b.bot.Send(msg); err != nil {
				log.Printf("Error sending welcome message: %v", err)
			}

			// Check if user already has a timezone set
			prefs := b.getUserPreferences(userID)
			if prefs.Timezone == "UTC" {
				// Ask user to set their timezone
				timezoneRequestMsg := tgbotapi.NewMessage(chatID, "To provide accurate calendar events, I need to know your timezone. Please set it using the /timezone command followed by your timezone.\n\nExamples:\n/timezone Europe/London\n/timezone America/New_York\n/timezone Asia/Tokyo\n/timezone GMT+3\n/timezone GMT-5:30")

				// Add a custom keyboard with common timezones
				keyboard := b.createTimezoneKeyboard()
				timezoneRequestMsg.ReplyMarkup = keyboard

				if _, err := b.bot.Send(timezoneRequestMsg); err != nil {
					log.Printf("Error sending timezone request message: %v", err)
				}
			} else {
				// User already has a timezone set, just send the help message
				b.handleHelp(chatID, messageID)
			}
			return
		case "clear":
			// Clear the thread for this user
			if err := b.openaiClient.ClearThreadForUser(ctx, userID); err != nil {
				log.Printf("Error clearing thread for user %s: %v", userID, err)
				b.sendErrorMessage(chatID, fmt.Errorf("failed to clear thread: %w", err), messageID)
				return
			}
			msg := tgbotapi.NewMessage(chatID, "Your conversation history has been cleared.")
			msg.ReplyToMessageID = messageID // Reply to the original message
			if _, err := b.bot.Send(msg); err != nil {
				log.Printf("Error sending clear confirmation: %v", err)
			}
			return
		case "timezone":
			// Set the user's timezone
			args := message.CommandArguments()
			if args == "" {
				// If no timezone provided, show the current timezone
				prefs := b.getUserPreferences(userID)
				msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Your current timezone is set to: %s\n\nTo change it, use /timezone followed by an IANA timezone name or GMT offset, for example:\n/timezone Europe/London\n/timezone America/New_York\n/timezone Asia/Tokyo\n/timezone GMT+3\n/timezone GMT-5:30", b.formatTimezoneForDisplay(prefs.Timezone)))
				msg.ReplyToMessageID = messageID

				// Add a custom keyboard with common timezones
				keyboard := b.createTimezoneKeyboard()
				msg.ReplyMarkup = keyboard

				if _, err := b.bot.Send(msg); err != nil {
					log.Printf("Error sending timezone info: %v", err)
				}
				return
			}

			// Validate and set the timezone
			timezone, err := b.parseTimezone(args)
			if err != nil {
				msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Invalid timezone: %s\n\nPlease use a valid IANA timezone name or GMT offset, for example:\n/timezone Europe/London\n/timezone America/New_York\n/timezone Asia/Tokyo\n/timezone GMT+3\n/timezone GMT-5:30", args))
				msg.ReplyToMessageID = messageID
				if _, err := b.bot.Send(msg); err != nil {
					log.Printf("Error sending timezone error: %v", err)
				}
				return
			}

			// Set the timezone
			b.setUserTimezone(userID, timezone)
			msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Your timezone has been set to: %s", b.formatTimezoneForDisplay(timezone)))
			msg.ReplyToMessageID = messageID

			// Remove the custom keyboard
			msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)

			if _, err := b.bot.Send(msg); err != nil {
				log.Printf("Error sending timezone confirmation: %v", err)
			}
			return
		case "help":
			b.handleHelp(chatID, messageID)
			return
		case "refresh_commands":
			// Only allow admin to refresh commands
			if b.isAdmin(userID) {
				if err := b.setupCommands(); err != nil {
					b.sendErrorMessage(chatID, fmt.Errorf("failed to refresh commands: %w", err), messageID)
					return
				}
				msg := tgbotapi.NewMessage(chatID, "Bot commands have been refreshed successfully.")
				msg.ReplyToMessageID = messageID
				if _, err := b.bot.Send(msg); err != nil {
					log.Printf("Error sending refresh confirmation: %v", err)
				}
			} else {
				b.sendErrorMessage(chatID, fmt.Errorf("you are not authorized to use this command"), messageID)
			}
			return
		}
	}

	// Check if user has set a timezone
	prefs := b.getUserPreferences(userID)
	if prefs.Timezone == "UTC" && !message.IsCommand() {
		// User hasn't set a timezone and is trying to create an event
		timezoneRequestMsg := tgbotapi.NewMessage(chatID, "Before I can process your event, I need to know your timezone. Please set it using the /timezone command followed by your timezone.\n\nExamples:\n/timezone Europe/London\n/timezone America/New_York\n/timezone Asia/Tokyo\n/timezone GMT+3\n/timezone GMT-5:30")

		// Add a custom keyboard with common timezones
		keyboard := b.createTimezoneKeyboard()
		timezoneRequestMsg.ReplyMarkup = keyboard
		timezoneRequestMsg.ReplyToMessageID = messageID

		if _, err := b.bot.Send(timezoneRequestMsg); err != nil {
			log.Printf("Error sending timezone request message: %v", err)
		}
		return
	}

	// Send a "processing" message
	processingMsg := tgbotapi.NewMessage(chatID, "Processing your request...")
	processingMsg.ReplyToMessageID = messageID // Reply to the original message
	sentMsg, err := b.bot.Send(processingMsg)
	if err != nil {
		log.Printf("Error sending processing message: %v", err)
	} else {
		log.Printf("Sent processing message with ID: %d", sentMsg.MessageID)
	}

	var event *openai.Event
	var extractErr error

	// Handle text message
	if message.Text != "" {
		log.Printf("Processing text message: %s", message.Text)
		event, extractErr = b.openaiClient.ExtractEventFromText(ctx, userID, message.Text)
		if extractErr != nil {
			log.Printf("Error extracting event from text: %v", extractErr)
		} else {
			log.Printf("Successfully extracted event from text: %+v", event)
		}
	}

	// Handle photo
	if message.Photo != nil && len(message.Photo) > 0 {
		log.Printf("Processing photo message with %d photos", len(message.Photo))
		// Get the largest photo
		photo := message.Photo[len(message.Photo)-1]
		log.Printf("Using largest photo with file ID: %s", photo.FileID)

		// Get file URL
		fileURL, err := b.bot.GetFileDirectURL(photo.FileID)
		if err != nil {
			log.Printf("Error getting photo URL: %v", err)
			b.sendErrorMessage(chatID, fmt.Errorf("failed to get photo URL: %w", err), messageID)
			return
		}
		log.Printf("Got file URL: %s", fileURL)

		// Download the photo
		imageData, err := b.downloadFile(fileURL)
		if err != nil {
			log.Printf("Error downloading photo: %v", err)
			b.sendErrorMessage(chatID, fmt.Errorf("failed to download photo: %w", err), messageID)
			return
		}
		log.Printf("Downloaded photo, size: %d bytes", len(imageData))

		// Extract event from image
		event, extractErr = b.openaiClient.ExtractEventFromImage(ctx, userID, imageData)
		if extractErr != nil {
			log.Printf("Error extracting event from image: %v", extractErr)
		} else {
			log.Printf("Successfully extracted event from image: %+v", event)
		}
	}

	// Handle document (for screenshots sent as files)
	if message.Document != nil {
		log.Printf("Processing document with MIME type: %s", message.Document.MimeType)
		// Check if it's an image
		if isImageMIME(message.Document.MimeType) {
			log.Printf("Document is an image, processing...")
			// Get file URL
			fileURL, err := b.bot.GetFileDirectURL(message.Document.FileID)
			if err != nil {
				log.Printf("Error getting document URL: %v", err)
				b.sendErrorMessage(chatID, fmt.Errorf("failed to get document URL: %w", err), messageID)
				return
			}
			log.Printf("Got document URL: %s", fileURL)

			// Download the document
			imageData, err := b.downloadFile(fileURL)
			if err != nil {
				log.Printf("Error downloading document: %v", err)
				b.sendErrorMessage(chatID, fmt.Errorf("failed to download document: %w", err), messageID)
				return
			}
			log.Printf("Downloaded document, size: %d bytes", len(imageData))

			// Extract event from image
			event, extractErr = b.openaiClient.ExtractEventFromImage(ctx, userID, imageData)
			if extractErr != nil {
				log.Printf("Error extracting event from document: %v", extractErr)
			} else {
				log.Printf("Successfully extracted event from document: %+v", event)
			}
		} else {
			log.Printf("Unsupported document type: %s", message.Document.MimeType)
			b.sendErrorMessage(chatID, fmt.Errorf("unsupported document type: %s", message.Document.MimeType), messageID)
			return
		}
	}

	// Handle extraction error
	if extractErr != nil {
		log.Printf("Extraction error: %v", extractErr)
		b.sendErrorMessage(chatID, fmt.Errorf("failed to extract event: %w", extractErr), messageID)
		return
	}

	// If no event was extracted
	if event == nil {
		log.Println("No event information found")
		b.sendErrorMessage(chatID, fmt.Errorf("no event information found"), messageID)
		return
	}

	// Get user preferences for timezone
	prefs = b.getUserPreferences(userID)
	log.Printf("Using timezone %s for user %s", prefs.Timezone, userID)

	// Load the user's timezone
	loc, err := time.LoadLocation(prefs.Timezone)
	if err != nil {
		log.Printf("Error loading timezone %s: %v, falling back to UTC", prefs.Timezone, err)
		loc = time.UTC
	}

	// Convert event times to user's timezone
	log.Printf("Original UTC start time: %s", event.StartTime.Format(time.RFC3339))
	log.Printf("Original UTC end time: %s", event.EndTime.Format(time.RFC3339))

	localStartTime := event.StartTime.In(loc)
	localEndTime := event.EndTime.In(loc)

	log.Printf("Converted to timezone %s - start time: %s", prefs.Timezone, localStartTime.Format(time.RFC3339))
	log.Printf("Converted to timezone %s - end time: %s", prefs.Timezone, localEndTime.Format(time.RFC3339))

	// Determine if it's an all-day event
	eventType := "Timed event"
	startTimeFormat := "2006-01-02 15:04"
	endTimeFormat := "2006-01-02 15:04"

	// Check if it's an all-day event based on the original UTC time
	isAllDay := event.StartTime.Hour() == 0 && event.StartTime.Minute() == 0 && event.StartTime.Second() == 0

	if isAllDay {
		eventType = "All-day event"
		startTimeFormat = "2006-01-02"
		endTimeFormat = "2006-01-02"

		// For all-day events, we want to show the date without time
		// regardless of the timezone conversion
		log.Println("All-day event detected, using date-only format")
	}

	// Generate ICS file
	log.Println("Generating ICS file...")
	icsData, err := calendar.GenerateICS(event, prefs.Timezone)
	if err != nil {
		log.Printf("Error generating ICS file: %v", err)
		b.sendErrorMessage(chatID, fmt.Errorf("failed to generate ICS file: %w", err), messageID)
		return
	}
	log.Printf("Generated ICS file, size: %d bytes", len(icsData))

	// Create a temporary file for the ICS
	tempDir := os.TempDir()
	tempFile := filepath.Join(tempDir, fmt.Sprintf("event_%d.ics", messageID))
	log.Printf("Creating temporary file: %s", tempFile)

	if err := os.WriteFile(tempFile, icsData, 0644); err != nil {
		log.Printf("Error saving ICS file: %v", err)
		b.sendErrorMessage(chatID, fmt.Errorf("failed to save ICS file: %w", err), messageID)
		return
	}
	defer os.Remove(tempFile)

	// Send the ICS file
	log.Println("Sending ICS file...")
	doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(tempFile))

	doc.Caption = fmt.Sprintf("%s: %s\nStart: %s\nEnd: %s\nLocation: %s\nTimezone: %s\n\n📱 iPhone users: Use this shortcut for easy calendar import:\nhttps://www.icloud.com/shortcuts/db9d3a471c414a1abd2ba7b960395bee",
		eventType,
		event.Title,
		localStartTime.Format(startTimeFormat),
		localEndTime.Format(endTimeFormat),
		event.Location,
		b.formatTimezoneForDisplay(prefs.Timezone))
	doc.ReplyToMessageID = messageID // Reply to the original message

	// Delete the processing message
	log.Printf("Deleting processing message with ID: %d", sentMsg.MessageID)
	deleteMsg := tgbotapi.NewDeleteMessage(chatID, sentMsg.MessageID)
	if _, err := b.bot.Request(deleteMsg); err != nil {
		log.Printf("Error deleting processing message: %v", err)
	}

	if _, err := b.bot.Send(doc); err != nil {
		log.Printf("Error sending ICS file: %v", err)
		b.sendErrorMessage(chatID, fmt.Errorf("failed to send ICS file: %w", err), messageID)
		return
	}
	log.Println("ICS file sent successfully")
}

// sendErrorMessage sends an error message to the user
func (b *Bot) sendErrorMessage(chatID int64, err error, messageID int) {
	log.Printf("Sending error message to chat ID %d: %v", chatID, err)
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Error: %v", err))
	msg.ReplyToMessageID = messageID // Reply to the original message
	if _, err := b.bot.Send(msg); err != nil {
		log.Printf("Error sending error message: %v", err)
	}
}

// downloadFile downloads a file from a URL
func (b *Bot) downloadFile(url string) ([]byte, error) {
	log.Printf("Downloading file from URL: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	log.Printf("Downloaded %d bytes", len(data))
	return data, nil
}

// isImageMIME checks if a MIME type is an image
func isImageMIME(mimeType string) bool {
	imageMIMEs := map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/gif":  true,
		"image/webp": true,
		"image/bmp":  true,
	}

	return imageMIMEs[mimeType]
}

// parseTimezone handles both IANA timezone names and GMT offsets
func (b *Bot) parseTimezone(timezoneStr string) (string, error) {
	// First, check if it's a valid IANA timezone
	_, err := time.LoadLocation(timezoneStr)
	if err == nil {
		return timezoneStr, nil
	}

	// Check if it's a GMT offset format
	timezoneStr = strings.TrimSpace(timezoneStr)

	// Handle just "GMT" or "UTC" (equivalent to GMT+0/UTC+0)
	if strings.EqualFold(timezoneStr, "GMT") || strings.EqualFold(timezoneStr, "UTC") {
		return "UTC", nil
	}

	// Handle various GMT formats: GMT+3, GMT-5, GMT +3, GMT -5, UTC+3, UTC-5, etc.
	gmtPattern := regexp.MustCompile(`^(?i)(GMT|UTC)\s*([+-])\s*(\d+)(?::(\d+))?$`)
	matches := gmtPattern.FindStringSubmatch(timezoneStr)

	if matches != nil {
		// Extract hours and minutes
		sign := matches[2]
		hours, _ := strconv.Atoi(matches[3])

		// Validate hours
		if hours > 14 {
			return "", fmt.Errorf("invalid GMT offset: hours must be between 0 and 14")
		}

		// Handle minutes if present
		minutes := 0
		if matches[4] != "" {
			minutes, _ = strconv.Atoi(matches[4])
			if minutes >= 60 || minutes < 0 {
				return "", fmt.Errorf("invalid GMT offset: minutes must be between 0 and 59")
			}

			// If we have non-zero minutes, we need to convert to a more specific format
			// However, Etc/GMT only supports whole hour offsets, so we'll round to the nearest hour
			if minutes >= 30 {
				hours++
			}
		}

		// For Etc/GMT format, the sign is inverted (Etc/GMT+1 is actually GMT-1)
		// So we need to flip the sign
		var invertedSign string
		if sign == "+" {
			invertedSign = "-"
		} else {
			invertedSign = "+"
		}

		return fmt.Sprintf("Etc/GMT%s%d", invertedSign, hours), nil
	}

	return "", fmt.Errorf("invalid timezone format. Please use an IANA timezone name (e.g., 'Europe/London') or GMT offset (e.g., 'GMT+3')")
}

// formatTimezoneForDisplay formats a timezone for display to the user
func (b *Bot) formatTimezoneForDisplay(timezone string) string {
	// If it's an Etc/GMT timezone, convert it to a more user-friendly format
	if strings.HasPrefix(timezone, "Etc/GMT") {
		// Extract the offset
		offsetStr := strings.TrimPrefix(timezone, "Etc/GMT")
		if offsetStr == "" {
			return "GMT+0"
		}

		// Etc/GMT uses opposite sign, so we need to flip it
		if offsetStr[0] == '+' {
			return "GMT-" + offsetStr[1:]
		} else if offsetStr[0] == '-' {
			return "GMT+" + offsetStr[1:]
		}
		return "GMT" + offsetStr
	}

	// For IANA timezones, just return as is
	return timezone
}

// handleHelp sends a help message to the user
func (b *Bot) handleHelp(chatID int64, messageID int) {
	// Get the user's current timezone
	userID := fmt.Sprintf("%d", chatID) // Use the chat ID as the user ID for simplicity
	prefs := b.getUserPreferences(userID)
	timezoneInfo := fmt.Sprintf("Your current timezone is set to: %s", b.formatTimezoneForDisplay(prefs.Timezone))

	if prefs.Timezone == "UTC" {
		timezoneInfo += " (default)\n⚠️ It's important to set your correct timezone for accurate calendar events!"
	}

	helpText := fmt.Sprintf(`Calendar Assistant Bot Help:

%s

Send me a photo of an event announcement or a text description of an event, and I'll create a calendar file (.ics) that you can import into your calendar app.

Commands:
/start - Start the bot
/help - Show this help message
/timezone - View or set your timezone
  Examples:
    /timezone - Show your current timezone
    /timezone Europe/London - Set timezone to London
    /timezone America/New_York - Set timezone to New York
    /timezone GMT+3 - Set timezone to GMT+3
    /timezone GMT-5:30 - Set timezone to GMT-5:30
/clear - Clear your conversation history

Tip: You can see all available commands by typing "/" in the chat - Telegram will show command autocompletions.

When you send me an event, I'll extract:
- Event title
- Description
- Location
- Start time
- End time

The calendar file will be created in your preferred timezone. If no timezone is set, UTC will be used.

To import the .ics file:
- On iOS: Open the file to add it to your Calendar
  📱 For easier iPhone setup: Use this shortcut to automatically add .ics files to your calendar:
  https://www.icloud.com/shortcuts/db9d3a471c414a1abd2ba7b960395bee
- On Android: Open the file with your calendar app
- On desktop: Double-click the file or import it through your calendar application`, timezoneInfo)

	msg := tgbotapi.NewMessage(chatID, helpText)
	msg.ReplyToMessageID = messageID

	// If timezone is not set, add the timezone keyboard
	if prefs.Timezone == "UTC" {
		msg.ReplyMarkup = b.createTimezoneKeyboard()
	}

	if _, err := b.bot.Send(msg); err != nil {
		log.Printf("Error sending help message: %v", err)
	}
}

// isAdmin checks if a user is an admin
func (b *Bot) isAdmin(userID string) bool {
	// For now, let's consider all users as admins for the refresh_commands command
	// In a production environment, you would want to check against a list of admin user IDs
	return true
}

// createTimezoneKeyboard creates a keyboard with common timezone options
func (b *Bot) createTimezoneKeyboard() tgbotapi.ReplyKeyboardMarkup {
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("/timezone GMT+0"),
			tgbotapi.NewKeyboardButton("/timezone GMT+1"),
			tgbotapi.NewKeyboardButton("/timezone GMT+2"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("/timezone GMT+3"),
			tgbotapi.NewKeyboardButton("/timezone GMT+4"),
			tgbotapi.NewKeyboardButton("/timezone GMT+5"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("/timezone GMT-5"),
			tgbotapi.NewKeyboardButton("/timezone GMT-8"),
			tgbotapi.NewKeyboardButton("/timezone GMT-10"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("/timezone Europe/London"),
			tgbotapi.NewKeyboardButton("/timezone America/New_York"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("/timezone Asia/Tokyo"),
			tgbotapi.NewKeyboardButton("/timezone Australia/Sydney"),
		),
	)
	keyboard.OneTimeKeyboard = true
	return keyboard
}

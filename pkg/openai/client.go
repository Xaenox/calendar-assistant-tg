package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"calendar-assistant/pkg/config"
)

// Client represents an OpenAI API client
type Client struct {
	client        *openai.Client
	assistantID   string
	assistantName string
	threadCache   map[string]string // Map of userID -> threadID
	cacheMutex    sync.RWMutex      // Mutex to protect the thread cache
}

// Event represents a calendar event
type Event struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Location    string    `json:"location"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
}

// NewClient creates a new OpenAI client
func NewClient(cfg *config.Config) *Client {
	// Set the beta header for assistants API v2
	betaOption := option.WithHeader("OpenAI-Beta", "assistants=v2")
	apiKeyOption := option.WithAPIKey(cfg.OpenAIAPIKey)
	client := openai.NewClient(betaOption, apiKeyOption)

	// Default assistant name - can be configured if needed
	assistantName := "Calendar Assistant"

	return &Client{
		client:        client,
		assistantID:   cfg.OpenAIAssistantID,
		assistantName: assistantName,
		threadCache:   make(map[string]string),
	}
}

// getOrCreateThread gets an existing thread for a user or creates a new one
func (c *Client) getOrCreateThread(ctx context.Context, userID string) (string, error) {
	// Check if we have a cached thread for this user
	c.cacheMutex.RLock()
	threadID, exists := c.threadCache[userID]
	c.cacheMutex.RUnlock()

	if exists {
		fmt.Printf("Using cached thread %s for user %s\n", threadID, userID)
		// Verify that the thread still exists
		_, err := c.client.Beta.Threads.Get(ctx, threadID)
		if err == nil {
			// Thread exists, we can use it
			return threadID, nil
		}
		fmt.Printf("Cached thread %s for user %s no longer exists: %v\n", threadID, userID, err)
		// If there's an error, the thread might not exist, so we'll create a new one
	}

	// Create a new thread
	fmt.Printf("Creating a new thread for user %s\n", userID)
	thread, err := c.client.Beta.Threads.New(ctx, openai.BetaThreadNewParams{})
	if err != nil {
		return "", fmt.Errorf("failed to create thread: %w", err)
	}

	// Cache the thread ID
	c.cacheMutex.Lock()
	c.threadCache[userID] = thread.ID
	c.cacheMutex.Unlock()

	fmt.Printf("Created and cached thread %s for user %s\n", thread.ID, userID)
	return thread.ID, nil
}

// InitializeAssistant creates or retrieves the assistant
func (c *Client) InitializeAssistant(ctx context.Context) error {
	// Check if we already have an assistant ID
	if c.assistantID != "" {
		// Verify that the assistant exists
		_, err := c.client.Beta.Assistants.Get(ctx, c.assistantID)
		if err == nil {
			// Assistant exists, we can use it
			return nil
		}
		// If there's an error, the assistant might not exist, so we'll create a new one
		fmt.Printf("Assistant ID from configuration not found: %v\n", err)
		c.assistantID = "" // Reset the ID so we can create a new one
	}

	// List assistants to find one with our name
	// Note: The API doesn't support filtering by name, so we need to list and filter manually
	assistants, err := c.client.Beta.Assistants.List(ctx, openai.BetaAssistantListParams{})
	if err != nil {
		return fmt.Errorf("failed to list assistants: %w", err)
	}

	// Look for an assistant with the matching name
	for _, assistant := range assistants.Data {
		if assistant.Name == c.assistantName {
			c.assistantID = assistant.ID
			return nil
		}
	}

	return nil
}

// formatCurrentDate returns the current date in a user-friendly format
func formatCurrentDate() string {
	now := time.Now()
	return fmt.Sprintf("%s, %s %d, %d",
		now.Weekday().String(),
		now.Month().String(),
		now.Day(),
		now.Year())
}

// ExtractEventFromText extracts event information from text
func (c *Client) ExtractEventFromText(ctx context.Context, userID string, text string) (*Event, error) {
	// Initialize assistant if needed
	if err := c.InitializeAssistant(ctx); err != nil {
		return nil, err
	}

	// Get or create a thread for this user
	threadID, err := c.getOrCreateThread(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Add current date information to the message
	currentDate := formatCurrentDate()
	messageText := fmt.Sprintf("Today is %s. Please extract event information from the following text:\n\n%s", currentDate, text)

	fmt.Printf("Sending message with current date: %s\n", currentDate)

	// Add a message to the thread
	role := openai.BetaThreadMessageNewParamsRoleUser
	_, err = c.client.Beta.Threads.Messages.New(ctx, threadID, openai.BetaThreadMessageNewParams{
		Role: openai.F(role),
		Content: openai.F([]openai.MessageContentPartParamUnion{
			openai.TextContentBlockParam{
				Type: openai.F(openai.TextContentBlockParamTypeText),
				Text: openai.String(messageText),
			},
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create message: %w", err)
	}

	// Run the assistant
	run, err := c.client.Beta.Threads.Runs.New(ctx, threadID, openai.BetaThreadRunNewParams{
		AssistantID: openai.F(c.assistantID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create run: %w", err)
	}

	// Poll for completion
	event, err := c.pollForCompletion(ctx, threadID, run.ID)
	if err != nil {
		return nil, err
	}

	return event, nil
}

// ExtractEventFromImage extracts event information from an image
func (c *Client) ExtractEventFromImage(ctx context.Context, userID string, imageData []byte) (*Event, error) {
	// Initialize assistant if needed
	if err := c.InitializeAssistant(ctx); err != nil {
		return nil, err
	}

	// Get or create a thread for this user
	threadID, err := c.getOrCreateThread(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Create a temporary file with a proper extension
	tempFile, err := os.CreateTemp("", "event-image-*.png")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tempFile.Name()) // Clean up the file when we're done

	// Write the image data to the temporary file
	if _, err := tempFile.Write(imageData); err != nil {
		tempFile.Close()
		return nil, fmt.Errorf("failed to write image data to temporary file: %w", err)
	}
	tempFile.Close()

	// Reopen the file for reading
	file, err := os.Open(tempFile.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to open temporary file: %w", err)
	}
	defer file.Close()

	// Upload the image as a file
	fmt.Printf("Uploading image file: %s with purpose: %s\n", tempFile.Name(), openai.FilePurposeVision)
	fileObj, err := c.client.Files.New(ctx, openai.FileNewParams{
		File:    openai.F[io.Reader](file),
		Purpose: openai.F(openai.FilePurposeVision),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upload image: %w", err)
	}

	// Print file information for debugging
	fmt.Printf("Uploaded file with ID: %s, Filename: %s, Purpose: %s\n",
		fileObj.ID, tempFile.Name(), fileObj.Purpose)

	// Add a message with the image to the thread
	role := openai.BetaThreadMessageNewParamsRoleUser
	fmt.Println("Creating message with image content...")

	// Add current date information to the message
	currentDate := formatCurrentDate()
	messageText := fmt.Sprintf("Today is %s. Please extract event information from this image.", currentDate)

	fmt.Printf("Sending message with current date: %s\n", currentDate)

	// Create the message with image content
	message, err := c.client.Beta.Threads.Messages.New(ctx, threadID, openai.BetaThreadMessageNewParams{
		Role: openai.F(role),
		Content: openai.F([]openai.MessageContentPartParamUnion{
			openai.TextContentBlockParam{
				Type: openai.F(openai.TextContentBlockParamTypeText),
				Text: openai.F(messageText),
			},
			openai.ImageFileContentBlockParam{
				Type: openai.F(openai.ImageFileContentBlockTypeImageFile),
				ImageFile: openai.F(openai.ImageFileParam{
					FileID: openai.F(fileObj.ID),
					Detail: openai.F(openai.ImageFileDetailHigh),
				}),
			},
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create message with image: %w", err)
	}
	fmt.Printf("Created message with ID: %s\n", message.ID)

	// Run the assistant
	fmt.Printf("Running assistant with ID: %s on thread: %s\n", c.assistantID, threadID)
	run, err := c.client.Beta.Threads.Runs.New(ctx, threadID, openai.BetaThreadRunNewParams{
		AssistantID: openai.F(c.assistantID),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create run: %w", err)
	}
	fmt.Printf("Created run with ID: %s\n", run.ID)

	// Poll for completion
	event, err := c.pollForCompletion(ctx, threadID, run.ID)
	if err != nil {
		return nil, err
	}

	return event, nil
}

// ClearThreadForUser clears the thread for a specific user
func (c *Client) ClearThreadForUser(ctx context.Context, userID string) error {
	c.cacheMutex.RLock()
	threadID, exists := c.threadCache[userID]
	c.cacheMutex.RUnlock()

	if !exists {
		return nil // No thread to clear
	}

	// Delete the thread from the cache
	c.cacheMutex.Lock()
	delete(c.threadCache, userID)
	c.cacheMutex.Unlock()

	fmt.Printf("Cleared thread %s for user %s from cache\n", threadID, userID)
	return nil
}

// pollForCompletion polls for the completion of a run and extracts the event information
func (c *Client) pollForCompletion(ctx context.Context, threadID, runID string) (*Event, error) {
	fmt.Printf("Starting to poll for completion of run %s on thread %s\n", runID, threadID)
	pollCount := 0

	// Poll for completion
	for {
		pollCount++
		fmt.Printf("Poll attempt #%d for run %s\n", pollCount, runID)

		run, err := c.client.Beta.Threads.Runs.Get(ctx, threadID, runID)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve run: %w", err)
		}

		fmt.Printf("Run status: %s\n", run.Status)

		switch run.Status {
		case openai.RunStatusCompleted:
			fmt.Println("Run completed successfully, retrieving messages...")
			// Get the messages
			order := openai.BetaThreadMessageListParamsOrderDesc
			messages, err := c.client.Beta.Threads.Messages.List(ctx, threadID, openai.BetaThreadMessageListParams{
				Order: openai.F(order),
				Limit: openai.F(int64(1)),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to list messages: %w", err)
			}

			fmt.Printf("Retrieved %d messages\n", len(messages.Data))

			if len(messages.Data) == 0 {
				return nil, fmt.Errorf("no messages found")
			}

			// Extract the event information from the assistant's response
			assistantMessage := messages.Data[0]
			if assistantMessage.Role != openai.MessageRoleAssistant {
				return nil, fmt.Errorf("unexpected message role: %s", assistantMessage.Role)
			}

			// Extract JSON from the message content
			var jsonContent string
			for _, content := range assistantMessage.Content {
				// Check the type of content
				if content.Type == openai.MessageContentTypeText {
					// Access the text content
					jsonContent = content.Text.Value
					break
				}
			}

			if jsonContent == "" {
				return nil, fmt.Errorf("no text content found in assistant message")
			}

			// Log the full response from the assistant
			fmt.Println("=== ASSISTANT RESPONSE ===")
			fmt.Println(jsonContent)
			fmt.Println("=========================")

			// Parse the JSON
			var eventData struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				Location    string `json:"location"`
				StartTime   string `json:"start_time"`
				EndTime     string `json:"end_time"`
			}

			// Try to extract JSON from the text
			// Look for JSON object markers
			startIdx := bytes.IndexByte([]byte(jsonContent), '{')
			endIdx := bytes.LastIndexByte([]byte(jsonContent), '}')

			if startIdx >= 0 && endIdx > startIdx {
				fmt.Printf("Found JSON object from index %d to %d\n", startIdx, endIdx)
				jsonContent = jsonContent[startIdx : endIdx+1]
				fmt.Printf("Extracted JSON: %s\n", jsonContent)
			} else {
				fmt.Println("Warning: Could not find JSON object markers in the response")
			}

			if err := json.Unmarshal([]byte(jsonContent), &eventData); err != nil {
				fmt.Printf("JSON unmarshal error: %v\n", err)
				return nil, fmt.Errorf("failed to parse event data: %w", err)
			}

			// Print the extracted data for debugging
			fmt.Printf("Extracted event data: %+v\n", eventData)

			// Parse the times with fallback to current time if empty or invalid
			var startTime, endTime time.Time
			now := time.Now()

			if eventData.StartTime == "" {
				startTime = now
				fmt.Println("Warning: Start time was empty, using current time")
			} else {
				var err error
				startTime, err = time.Parse(time.RFC3339, eventData.StartTime)
				if err != nil {
					fmt.Printf("Warning: Failed to parse start time '%s': %v, using current time\n",
						eventData.StartTime, err)
					startTime = now
				} else {
					// Check if this might be an all-day event (time at midnight)
					if startTime.Hour() == 0 && startTime.Minute() == 0 && startTime.Second() == 0 {
						fmt.Println("Detected possible all-day event (start time at midnight)")
					}
				}
			}

			if eventData.EndTime == "" {
				// Default to start time + 1 hour if end time is empty
				endTime = startTime.Add(1 * time.Hour)
				fmt.Println("Warning: End time was empty, using start time + 1 hour")

				// For all-day events, set end time to midnight of the next day
				if startTime.Hour() == 0 && startTime.Minute() == 0 && startTime.Second() == 0 {
					// Set to midnight of the next day
					endTime = time.Date(
						startTime.Year(), startTime.Month(), startTime.Day()+1,
						0, 0, 0, 0, startTime.Location(),
					)
					fmt.Println("All-day event detected, setting end time to midnight of the next day")
				}
			} else {
				var err error
				endTime, err = time.Parse(time.RFC3339, eventData.EndTime)
				if err != nil {
					fmt.Printf("Warning: Failed to parse end time '%s': %v, using start time + 1 hour\n",
						eventData.EndTime, err)
					endTime = startTime.Add(1 * time.Hour)

					// For all-day events, set end time to midnight of the next day
					if startTime.Hour() == 0 && startTime.Minute() == 0 && startTime.Second() == 0 {
						// Set to midnight of the next day
						endTime = time.Date(
							startTime.Year(), startTime.Month(), startTime.Day()+1,
							0, 0, 0, 0, startTime.Location(),
						)
						fmt.Println("All-day event detected, setting end time to midnight of the next day")
					}
				}
			}

			return &Event{
				Title:       eventData.Title,
				Description: eventData.Description,
				Location:    eventData.Location,
				StartTime:   startTime,
				EndTime:     endTime,
			}, nil

		case openai.RunStatusFailed, openai.RunStatusCancelled, openai.RunStatusExpired:
			return nil, fmt.Errorf("run failed with status: %s", run.Status)

		case openai.RunStatusRequiresAction:
			// Handle required actions if needed
			return nil, fmt.Errorf("run requires action, not implemented")

		default:
			// Wait and check again
			time.Sleep(1 * time.Second)
		}
	}
}

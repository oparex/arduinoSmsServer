package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/gin-gonic/gin"
)

// SMSConnection interface for both real and mock connections
type SMSConnection interface {
	SendSMS(number, content string) error
	Close() error
	IsConnected() bool
}

// SMSRequest represents the incoming SMS request structure
type SMSRequest struct {
	Number  string `json:"number" binding:"required"`
	Content string `json:"content" binding:"required"`
}

// SMSResponse represents the API response
type SMSResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// SMSListResponse represents the response for listing received SMS
type SMSListResponse struct {
	Status   string        `json:"status"`
	Total    int           `json:"total"`
	Count    int           `json:"count"`
	Messages []ReceivedSMS `json:"messages"`
}

// SentSMSListResponse represents the response for listing sent SMS
type SentSMSListResponse struct {
	Status   string    `json:"status"`
	Total    int       `json:"total"`
	Count    int       `json:"count"`
	Messages []SentSMS `json:"messages"`
}

// App holds the application state
type App struct {
	db         *Database
	smsConn    SMSConnection
	deviceMode string
}

func main() {
	// Initialize database
	db, err := NewDatabase("./sms.db")
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	log.Println("Database initialized successfully")

	// Get device mode from environment
	deviceMode := GetDeviceMode()
	log.Printf("Device mode: %s", deviceMode)

	// Initialize connection to Arduino
	var smsConn SMSConnection

	if deviceMode == "mock" {
		log.Println("Using mock serial connection")
		smsConn = NewMockSerialConnection("/dev/ttyACM0")
	} else {
		// Auto-discover or use specific port
		var portName string

		if deviceMode == "auto" {
			log.Println("Auto-discovering Arduino device...")
			discoveredPort, err := DiscoverArduino()
			if err != nil {
				log.Printf("Arduino discovery failed: %v", err)
				log.Println("Falling back to mock mode")
				smsConn = NewMockSerialConnection("/dev/ttyACM0")
			} else {
				portName = discoveredPort
			}
		} else {
			portName = deviceMode
		}

		if portName != "" {
			arduinoConn, err := NewArduinoConnection(portName, db)
			if err != nil {
				log.Printf("Failed to connect to Arduino on %s: %v", portName, err)
				log.Println("Falling back to mock mode")
				smsConn = NewMockSerialConnection(portName)
			} else {
				smsConn = arduinoConn
				log.Printf("Successfully connected to Arduino on %s", portName)
			}
		}
	}

	defer smsConn.Close()

	// Create app instance
	app := &App{
		db:         db,
		smsConn:    smsConn,
		deviceMode: deviceMode,
	}

	// Create Gin router
	router := gin.Default()

	// Setup routes
	app.setupRoutes(router)

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting down...")
		smsConn.Close()
		db.Close()
		os.Exit(0)
	}()

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting Arduino SMS Server on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// setupRoutes configures all API routes
func (app *App) setupRoutes(router *gin.Engine) {
	// Health check endpoint
	router.GET("/health", app.healthCheck)

	// SMS sending endpoint
	router.POST("/send", app.sendSMS)

	// Get received SMS
	router.GET("/received", app.getReceivedSMS)

	// Get received SMS by number
	router.GET("/received/:number", app.getReceivedSMSByNumber)

	// Get sent SMS
	router.GET("/sent", app.getSentSMS)

	// Get sent SMS by number
	router.GET("/sent/:number", app.getSentSMSByNumber)

	// Get statistics
	router.GET("/stats", app.getStats)
}

// healthCheck returns the health status of the service
func (app *App) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"service":   "Arduino SMS Server",
		"connected": app.smsConn.IsConnected(),
		"mode":      app.deviceMode,
	})
}

// sendSMS handles SMS sending requests
func (app *App) sendSMS(c *gin.Context) {
	var req SMSRequest

	// Bind and validate JSON request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, SMSResponse{
			Status:  "error",
			Message: fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	// Validate phone number (basic validation)
	if len(req.Number) < 10 {
		c.JSON(http.StatusBadRequest, SMSResponse{
			Status:  "error",
			Message: "Invalid phone number (minimum 10 digits)",
		})
		return
	}

	// Validate content
	if len(req.Content) == 0 {
		c.JSON(http.StatusBadRequest, SMSResponse{
			Status:  "error",
			Message: "SMS content cannot be empty",
		})
		return
	}

	// Check if connected
	if !app.smsConn.IsConnected() {
		c.JSON(http.StatusServiceUnavailable, SMSResponse{
			Status:  "error",
			Message: "Not connected to Arduino device",
		})
		return
	}

	// Send SMS via serial connection
	err := app.smsConn.SendSMS(req.Number, req.Content)
	if err != nil {
		// Save failed SMS to database
		app.db.SaveSentSMS(req.Number, req.Content, "error", err.Error())

		c.JSON(http.StatusInternalServerError, SMSResponse{
			Status:  "error",
			Message: fmt.Sprintf("Failed to send SMS: %v", err),
		})
		return
	}

	// Save successful SMS to database
	if saveErr := app.db.SaveSentSMS(req.Number, req.Content, "success", ""); saveErr != nil {
		log.Printf("Failed to save sent SMS to database: %v", saveErr)
	}

	// Success response
	c.JSON(http.StatusOK, SMSResponse{
		Status:  "success",
		Message: fmt.Sprintf("SMS sent to %s", req.Number),
	})
}

// getReceivedSMS retrieves received SMS messages with pagination
func (app *App) getReceivedSMS(c *gin.Context) {
	// Parse query parameters
	limit := 50
	offset := 0

	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
			if limit > 100 {
				limit = 100 // Cap at 100
			}
		}
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Get messages from database
	messages, err := app.db.GetReceivedSMS(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, SMSResponse{
			Status:  "error",
			Message: fmt.Sprintf("Failed to retrieve messages: %v", err),
		})
		return
	}

	// Get total count
	total, err := app.db.CountReceivedSMS()
	if err != nil {
		total = 0
	}

	c.JSON(http.StatusOK, SMSListResponse{
		Status:   "success",
		Total:    total,
		Count:    len(messages),
		Messages: messages,
	})
}

// getReceivedSMSByNumber retrieves received SMS messages from a specific number
func (app *App) getReceivedSMSByNumber(c *gin.Context) {
	number := c.Param("number")

	// Parse query parameters
	limit := 50
	offset := 0

	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
			if limit > 100 {
				limit = 100
			}
		}
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Get messages from database
	messages, err := app.db.GetReceivedSMSByNumber(number, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, SMSResponse{
			Status:  "error",
			Message: fmt.Sprintf("Failed to retrieve messages: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, SMSListResponse{
		Status:   "success",
		Total:    len(messages),
		Count:    len(messages),
		Messages: messages,
	})
}

// getSentSMS retrieves sent SMS messages with pagination
func (app *App) getSentSMS(c *gin.Context) {
	// Parse query parameters
	limit := 50
	offset := 0

	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
			if limit > 100 {
				limit = 100 // Cap at 100
			}
		}
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Get messages from database
	messages, err := app.db.GetSentSMS(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, SMSResponse{
			Status:  "error",
			Message: fmt.Sprintf("Failed to retrieve messages: %v", err),
		})
		return
	}

	// Get total count
	total, err := app.db.CountSentSMS()
	if err != nil {
		total = 0
	}

	c.JSON(http.StatusOK, SentSMSListResponse{
		Status:   "success",
		Total:    total,
		Count:    len(messages),
		Messages: messages,
	})
}

// getSentSMSByNumber retrieves sent SMS messages to a specific number
func (app *App) getSentSMSByNumber(c *gin.Context) {
	number := c.Param("number")

	// Parse query parameters
	limit := 50
	offset := 0

	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
			if limit > 100 {
				limit = 100
			}
		}
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Get messages from database
	messages, err := app.db.GetSentSMSByNumber(number, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, SMSResponse{
			Status:  "error",
			Message: fmt.Sprintf("Failed to retrieve messages: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, SentSMSListResponse{
		Status:   "success",
		Total:    len(messages),
		Count:    len(messages),
		Messages: messages,
	})
}

// getStats returns statistics about the SMS gateway
func (app *App) getStats(c *gin.Context) {
	totalReceived, err := app.db.CountReceivedSMS()
	if err != nil {
		totalReceived = 0
	}

	totalSent, err := app.db.CountSentSMS()
	if err != nil {
		totalSent = 0
	}

	sentSuccess, err := app.db.CountSentSMSByStatus("success")
	if err != nil {
		sentSuccess = 0
	}

	sentError, err := app.db.CountSentSMSByStatus("error")
	if err != nil {
		sentError = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"status":         "success",
		"total_received": totalReceived,
		"total_sent":     totalSent,
		"sent_success":   sentSuccess,
		"sent_error":     sentError,
		"connected":      app.smsConn.IsConnected(),
		"mode":           app.deviceMode,
	})
}

package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

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

// MockSerialConnection simulates sending data to Arduino MKR GSM 1400
type MockSerialConnection struct {
	port string
}

// NewMockSerialConnection creates a new mock serial connection
func NewMockSerialConnection(port string) *MockSerialConnection {
	return &MockSerialConnection{
		port: port,
	}
}

// SendSMS simulates sending an SMS via the Arduino
func (m *MockSerialConnection) SendSMS(number, content string) error {
	log.Printf("[MOCK SERIAL] Port: %s", m.port)
	log.Printf("[MOCK SERIAL] Sending SMS to: %s", number)
	log.Printf("[MOCK SERIAL] Content: %s", content)

	// Simulate processing time
	time.Sleep(100 * time.Millisecond)

	log.Printf("[MOCK SERIAL] SMS sent successfully (simulated)")
	return nil
}

func main() {
	// Initialize mock serial connection
	serialConn := NewMockSerialConnection("/dev/ttyUSB0")

	// Create Gin router
	router := gin.Default()

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "healthy",
			"service": "Arduino SMS Server",
		})
	})

	// SMS sending endpoint
	router.POST("/send", func(c *gin.Context) {
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
				Message: "Invalid phone number",
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

		// Send SMS via mock serial connection
		err := serialConn.SendSMS(req.Number, req.Content)
		if err != nil {
			c.JSON(http.StatusInternalServerError, SMSResponse{
				Status:  "error",
				Message: fmt.Sprintf("Failed to send SMS: %v", err),
			})
			return
		}

		// Success response
		c.JSON(http.StatusOK, SMSResponse{
			Status:  "success",
			Message: fmt.Sprintf("SMS sent to %s", req.Number),
		})
	})

	// Start server
	port := ":8080"
	log.Printf("Starting Arduino SMS Server on port %s", port)
	log.Printf("Using mock serial connection on port: /dev/ttyUSB0")

	if err := router.Run(port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

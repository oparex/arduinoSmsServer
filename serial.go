package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
)

const (
	// Reconnection configuration
	reconnectInterval     = 5 * time.Second
	portClosedRetryDelay  = 1 * time.Second
	deviceModeAuto        = "auto"
	deviceModeMock        = "mock"
)

// SerialCommand represents a command to send to Arduino
type SerialCommand struct {
	Cmd     string `json:"cmd"`
	Number  string `json:"number,omitempty"`
	Content string `json:"content,omitempty"`
}

// SerialResponse represents a response from Arduino
type SerialResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Event   string `json:"event,omitempty"`
	Number  string `json:"number,omitempty"`
	Content string `json:"content,omitempty"`
	Time    string `json:"timestamp,omitempty"`
	GSM     string `json:"gsm,omitempty"`
}

// ArduinoConnection manages the serial connection to Arduino
type ArduinoConnection struct {
	port       serial.Port
	portName   string
	mu         sync.Mutex
	db         *Database
	connected  bool
	stopChan   chan bool
	stopOnce   sync.Once
	onReceived func(number, content string, timestamp time.Time)

	gsmReady   bool
	gsmMu      sync.RWMutex
	gsmWaiters []chan bool

	deviceMode      string
	reconnecting    bool
	reconnectMu     sync.Mutex
	shouldReconnect bool
}

// DiscoverArduino attempts to find the Arduino device on available serial ports
func DiscoverArduino() (string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return "", fmt.Errorf("failed to list serial ports: %w", err)
	}

	if len(ports) == 0 {
		return "", fmt.Errorf("no serial ports found")
	}

	// Common Arduino port patterns
	arduinoPatterns := []string{
		"/dev/ttyACM",  // Linux Arduino
		"/dev/ttyUSB",  // Linux USB-Serial
		"COM",          // Windows
		"/dev/cu.usb",  // macOS
		"/dev/tty.usb", // macOS
	}

	// Try to find Arduino on common ports
	for _, port := range ports {
		for _, pattern := range arduinoPatterns {
			if strings.Contains(port, pattern) {
				log.Printf("Found potential Arduino device: %s", port)

				// Try to open and test the connection
				if testSerialPort(port) {
					return port, nil
				}
			}
		}
	}

	// If no Arduino found by pattern, return the first available port
	if len(ports) > 0 {
		log.Printf("No Arduino pattern matched, trying first available port: %s", ports[0])
		if testSerialPort(ports[0]) {
			return ports[0], nil
		}
	}

	return "", fmt.Errorf("no Arduino device found on available ports: %v", ports)
}

// testSerialPort attempts to open and test a serial port
func testSerialPort(portName string) bool {
	mode := &serial.Mode{
		BaudRate: 115200,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(portName, mode)
	if err != nil {
		return false
	}
	defer port.Close()

	// Set timeouts
	port.SetReadTimeout(2 * time.Second)

	// Wait a moment for Arduino to initialize
	time.Sleep(500 * time.Millisecond)

	// Try to send a ping command
	_, err = port.Write([]byte("{\"cmd\":\"ping\"}\n"))
	if err != nil {
		return false
	}

	// Wait for response
	buf := make([]byte, 256)
	n, err := port.Read(buf)
	if err != nil || n == 0 {
		return false
	}

	log.Printf("Port %s responded: %s", portName, string(buf[:n]))
	return true
}

// NewArduinoConnection creates a new connection to Arduino
func NewArduinoConnection(portName string, db *Database) (*ArduinoConnection, error) {
	// Use auto mode as default for backward compatibility
	return NewArduinoConnectionWithMode(portName, db, deviceModeAuto)
}

// NewArduinoConnectionWithMode creates a new connection to Arduino with device mode for reconnection
func NewArduinoConnectionWithMode(portName string, db *Database, deviceMode string) (*ArduinoConnection, error) {
	mode := &serial.Mode{
		BaudRate: 115200,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(portName, mode)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port %s: %w", portName, err)
	}

	// Set read timeout
	err = port.SetReadTimeout(100 * time.Millisecond)
	if err != nil {
		port.Close()
		return nil, fmt.Errorf("failed to set read timeout: %w", err)
	}

	conn := &ArduinoConnection{
		port:            port,
		portName:        portName,
		db:              db,
		connected:       true,
		stopChan:        make(chan bool),
		deviceMode:      deviceMode,
		shouldReconnect: true,
	}

	// Wait for Arduino to initialize
	time.Sleep(2 * time.Second)

	// Start reading incoming messages
	go conn.readLoop()

	// Start periodic wakeup to check for received SMS
	go conn.periodicWakeup()

	log.Printf("Connected to Arduino on %s", portName)

	return conn, nil
}

// readLoop continuously reads from the serial port
func (a *ArduinoConnection) readLoop() {
	buf := make([]byte, 256)
	var lineBuf []byte

	for {
		select {
		case <-a.stopChan:
			return
		default:
			n, err := a.port.Read(buf)
			if err != nil {
				if !strings.Contains(err.Error(), "timeout") {
					// Check if this is a port closed or device disconnected error
					if isPortClosedError(err) {
						if a.connected {
							log.Printf("Arduino disconnected: %v", err)
							a.handleDisconnection()
						}
						// Wait before checking again to avoid tight loop
						time.Sleep(portClosedRetryDelay)
					} else if a.connected {
						log.Printf("Error reading from serial: %v", err)
					}
				}
				continue
			}
			if n == 0 {
				// Timeout with no data — this is normal, just loop
				continue
			}

			lineBuf = append(lineBuf, buf[:n]...)

			// Process complete lines
			for {
				idx := bytes.IndexByte(lineBuf, '\n')
				if idx < 0 {
					break
				}
				line := strings.TrimSpace(string(lineBuf[:idx]))
				lineBuf = lineBuf[idx+1:]

				if line == "" {
					continue
				}
				a.handleResponse(line)
			}
		}
	}
}

// isPortClosedError checks if the error indicates the port is closed or device disconnected
func isPortClosedError(err error) bool {
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "port has been closed") ||
		strings.Contains(errStr, "file already closed") ||
		strings.Contains(errStr, "device not configured") ||
		strings.Contains(errStr, "no such device") ||
		strings.Contains(errStr, "operation not permitted") ||
		strings.Contains(errStr, "i/o error") ||
		strings.Contains(errStr, "bad file descriptor")
}

// handleDisconnection handles when the Arduino is disconnected
func (a *ArduinoConnection) handleDisconnection() {
	a.mu.Lock()
	if !a.connected {
		a.mu.Unlock()
		return
	}
	a.connected = false
	a.mu.Unlock()

	// Update GSM state
	a.gsmMu.Lock()
	a.gsmReady = false
	a.gsmMu.Unlock()

	log.Println("Arduino connection lost, attempting to reconnect...")

	// Try to reconnect in the background
	if a.shouldReconnect {
		go a.attemptReconnection()
	}
}

// attemptReconnection tries to reconnect to Arduino
func (a *ArduinoConnection) attemptReconnection() {
	a.reconnectMu.Lock()
	if a.reconnecting {
		a.reconnectMu.Unlock()
		return
	}
	a.reconnecting = true
	a.reconnectMu.Unlock()

	defer func() {
		a.reconnectMu.Lock()
		a.reconnecting = false
		a.reconnectMu.Unlock()
	}()

	retryCount := 0

	for {
		if !a.shouldReconnect {
			log.Println("Reconnection disabled")
			return
		}

		// Check if we should stop - use select with default to avoid blocking
		select {
		case <-a.stopChan:
			log.Println("Reconnection cancelled - stop signal received")
			return
		default:
			// Continue with reconnection attempt
		}

		retryCount++
		log.Printf("Reconnection attempt #%d...", retryCount)

		var portName string
		var err error

		// Determine which port to try
		if a.deviceMode == deviceModeAuto || a.deviceMode == "" {
			// Auto-discovery mode: try to find Arduino
			portName, err = DiscoverArduino()
			if err != nil {
				log.Printf("Arduino discovery failed: %v", err)
				time.Sleep(reconnectInterval)
				continue
			}
		} else {
			// Specific port mode: try the same port
			portName = a.portName
		}

		// Try to reconnect
		if err := a.reconnect(portName); err != nil {
			log.Printf("Failed to reconnect to %s: %v", portName, err)
			time.Sleep(reconnectInterval)
			continue
		}

		log.Printf("Successfully reconnected to Arduino on %s", portName)
		return
	}
}

// reconnect attempts to reconnect to the specified port
func (a *ArduinoConnection) reconnect(portName string) error {
	mode := &serial.Mode{
		BaudRate: 115200,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(portName, mode)
	if err != nil {
		return fmt.Errorf("failed to open serial port %s: %w", portName, err)
	}

	// Set read timeout
	err = port.SetReadTimeout(100 * time.Millisecond)
	if err != nil {
		port.Close()
		return fmt.Errorf("failed to set read timeout: %w", err)
	}

	// Close old port if it exists
	a.mu.Lock()
	if a.port != nil {
		a.port.Close()
	}
	a.port = port
	a.portName = portName
	a.connected = true
	a.mu.Unlock()

	// Wait for Arduino to initialize
	time.Sleep(2 * time.Second)

	log.Printf("Reconnected to Arduino on %s", portName)

	return nil
}

// periodicWakeup wakes GSM once per hour to check for received SMS
func (a *ArduinoConnection) periodicWakeup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopChan:
			return
		case <-ticker.C:
			if !a.IsGSMReady() {
				log.Println("Periodic wakeup: connecting GSM to check for received SMS")
				if err := a.Wakeup(); err != nil {
					log.Printf("Periodic wakeup failed: %v", err)
				}
			}
		}
	}
}

// updateGSMState updates the GSM ready state and notifies waiters
func (a *ArduinoConnection) updateGSMState(state string) {
	a.gsmMu.Lock()
	defer a.gsmMu.Unlock()

	wasReady := a.gsmReady
	a.gsmReady = (state == "connected")

	if a.gsmReady == wasReady {
		return
	}

	log.Printf("GSM state changed: %s", state)

	if a.gsmReady {
		// Notify all waiters
		for _, ch := range a.gsmWaiters {
			select {
			case ch <- true:
			default:
			}
		}
		a.gsmWaiters = nil
	}
}

// IsGSMReady returns whether the GSM module is connected
func (a *ArduinoConnection) IsGSMReady() bool {
	a.gsmMu.RLock()
	defer a.gsmMu.RUnlock()
	return a.gsmReady
}

// WaitForGSM blocks until GSM is connected or timeout expires
func (a *ArduinoConnection) WaitForGSM(timeout time.Duration) bool {
	a.gsmMu.Lock()
	if a.gsmReady {
		a.gsmMu.Unlock()
		return true
	}

	ch := make(chan bool, 1)
	a.gsmWaiters = append(a.gsmWaiters, ch)
	a.gsmMu.Unlock()

	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}

// Wakeup sends a wakeup command to the Arduino
func (a *ArduinoConnection) Wakeup() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.connected {
		return fmt.Errorf("not connected to Arduino")
	}

	_, err := a.port.Write([]byte("{\"cmd\":\"wakeup\"}\n"))
	if err != nil {
		return fmt.Errorf("failed to send wakeup command: %w", err)
	}

	log.Println("Sent wakeup command to Arduino")
	return nil
}

// EnsureGSMReady wakes GSM if needed and waits for it to become ready
func (a *ArduinoConnection) EnsureGSMReady(timeout time.Duration) error {
	if a.IsGSMReady() {
		return nil
	}

	if err := a.Wakeup(); err != nil {
		return fmt.Errorf("failed to wake GSM: %w", err)
	}

	if !a.WaitForGSM(timeout) {
		return fmt.Errorf("GSM did not become ready within %v", timeout)
	}

	return nil
}

// handleResponse processes responses from Arduino
func (a *ArduinoConnection) handleResponse(line string) {
	var response SerialResponse

	err := json.Unmarshal([]byte(line), &response)
	if err != nil {
		log.Printf("Failed to parse Arduino response: %s (error: %v)", line, err)
		return
	}

	// Update GSM state from every response
	if response.GSM != "" {
		a.updateGSMState(response.GSM)
	}

	// Handle different response types
	switch {
	case response.Event == "gsm_state":
		// Already handled above via GSM field
		log.Printf("GSM state event: %s", response.GSM)

	case response.Event == "received":
		// Received SMS from Arduino
		log.Printf("Received SMS from %s: %s", response.Number, response.Content)
		a.handleReceivedSMS(response)

	case response.Status == "ready":
		log.Printf("Arduino ready: %s", response.Message)

	case response.Status == "info":
		log.Printf("Arduino info: %s", response.Message)

	case response.Status == "error":
		log.Printf("Arduino error: %s", response.Message)

	case response.Status == "ok":
		log.Printf("Arduino response: %s", response.Message)

	default:
		log.Printf("Unknown Arduino message: %s", line)
	}
}

// handleReceivedSMS processes a received SMS and stores it in the database
func (a *ArduinoConnection) handleReceivedSMS(response SerialResponse) {
	// Parse timestamp or use current time
	timestamp := time.Now()

	// Store in database
	if a.db != nil {
		err := a.db.SaveReceivedSMS(response.Number, response.Content, timestamp)
		if err != nil {
			log.Printf("Failed to save received SMS: %v", err)
		} else {
			log.Printf("Saved SMS from %s to database", response.Number)
		}
	}

	// Call callback if set
	if a.onReceived != nil {
		a.onReceived(response.Number, response.Content, timestamp)
	}
}

// SendSMS sends an SMS via the Arduino
func (a *ArduinoConnection) SendSMS(number, content string) error {
	// Ensure GSM is ready before sending
	if err := a.EnsureGSMReady(30 * time.Second); err != nil {
		return fmt.Errorf("GSM not ready: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.connected {
		return fmt.Errorf("not connected to Arduino")
	}

	cmd := SerialCommand{
		Cmd:     "send",
		Number:  number,
		Content: content,
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	// Add newline terminator
	data = append(data, '\n')

	// Write to serial port
	_, err = a.port.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write to serial port: %w", err)
	}

	log.Printf("Sent command to Arduino: %s", string(data))

	// Wait a bit for Arduino to process
	time.Sleep(100 * time.Millisecond)

	return nil
}

// Ping sends a ping command to Arduino
func (a *ArduinoConnection) Ping() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	cmd := SerialCommand{
		Cmd: "ping",
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	data = append(data, '\n')

	_, err = a.port.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write to serial port: %w", err)
	}

	return nil
}

// Close closes the serial connection
func (a *ArduinoConnection) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.shouldReconnect = false
	a.connected = false
	
	// Use sync.Once to ensure stopChan is closed only once
	a.stopOnce.Do(func() {
		close(a.stopChan)
	})

	if a.port != nil {
		return a.port.Close()
	}

	return nil
}

// IsConnected returns the connection status
func (a *ArduinoConnection) IsConnected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.connected
}

// MockSerialConnection simulates Arduino connection for testing
type MockSerialConnection struct {
	port string
}

// NewMockSerialConnection creates a mock connection
func NewMockSerialConnection(port string) *MockSerialConnection {
	return &MockSerialConnection{port: port}
}

// SendSMS simulates sending SMS
func (m *MockSerialConnection) SendSMS(number, content string) error {
	log.Printf("[MOCK] Sending SMS to %s: %s", number, content)
	time.Sleep(100 * time.Millisecond)
	return nil
}

// Close closes the mock connection
func (m *MockSerialConnection) Close() error {
	return nil
}

// IsConnected always returns true for mock
func (m *MockSerialConnection) IsConnected() bool {
	return true
}

// IsGSMReady always returns true for mock
func (m *MockSerialConnection) IsGSMReady() bool {
	return true
}

// Wakeup is a no-op for mock
func (m *MockSerialConnection) Wakeup() error {
	log.Println("[MOCK] Wakeup command (no-op)")
	return nil
}

// EnsureGSMReady is a no-op for mock
func (m *MockSerialConnection) EnsureGSMReady(timeout time.Duration) error {
	return nil
}

// GetDeviceMode returns the device connection mode from environment variable
func GetDeviceMode() string {
	mode := os.Getenv("DEVICE_MODE")
	if mode == "" {
		mode = "auto" // auto, mock, or specific port path
	}
	return mode
}

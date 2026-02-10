package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ReceivedSMS represents an SMS message received from the Arduino
type ReceivedSMS struct {
	ID        int       `json:"id"`
	Number    string    `json:"number"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	CreatedAt time.Time `json:"created_at"`
}

// SentSMS represents an SMS message sent via the Arduino
type SentSMS struct {
	ID        int       `json:"id"`
	Number    string    `json:"number"`
	Content   string    `json:"content"`
	Status    string    `json:"status"` // success, error
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Database handles SQLite operations
type Database struct {
	db *sql.DB
}

// NewDatabase creates a new database connection and initializes tables
func NewDatabase(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	database := &Database{db: db}

	// Initialize tables
	if err := database.initTables(); err != nil {
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}

	return database, nil
}

// initTables creates the necessary database tables
func (d *Database) initTables() error {
	query := `
	CREATE TABLE IF NOT EXISTS received_sms (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		number TEXT NOT NULL,
		content TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_received_sms_timestamp ON received_sms(timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_received_sms_number ON received_sms(number);

	CREATE TABLE IF NOT EXISTS sent_sms (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		number TEXT NOT NULL,
		content TEXT NOT NULL,
		status TEXT NOT NULL,
		error TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_sent_sms_created_at ON sent_sms(created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_sent_sms_number ON sent_sms(number);
	CREATE INDEX IF NOT EXISTS idx_sent_sms_status ON sent_sms(status);
	`

	_, err := d.db.Exec(query)
	return err
}

// SaveReceivedSMS stores a received SMS in the database
func (d *Database) SaveReceivedSMS(number, content string, timestamp time.Time) error {
	query := `INSERT INTO received_sms (number, content, timestamp) VALUES (?, ?, ?)`

	_, err := d.db.Exec(query, number, content, timestamp)
	if err != nil {
		return fmt.Errorf("failed to save SMS: %w", err)
	}

	return nil
}

// GetReceivedSMS retrieves all received SMS messages with pagination
func (d *Database) GetReceivedSMS(limit, offset int) ([]ReceivedSMS, error) {
	query := `
		SELECT id, number, content, timestamp, created_at
		FROM received_sms
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`

	rows, err := d.db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query SMS: %w", err)
	}
	defer rows.Close()

	var messages []ReceivedSMS

	for rows.Next() {
		var msg ReceivedSMS
		var timestampStr, createdAtStr string

		err := rows.Scan(&msg.ID, &msg.Number, &msg.Content, &timestampStr, &createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		msg.Timestamp = parseTimestamp(timestampStr)
		msg.CreatedAt = parseTimestamp(createdAtStr)

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return messages, nil
}

// GetReceivedSMSByNumber retrieves SMS messages from a specific number
func (d *Database) GetReceivedSMSByNumber(number string, limit, offset int) ([]ReceivedSMS, error) {
	query := `
		SELECT id, number, content, timestamp, created_at
		FROM received_sms
		WHERE number = ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`

	rows, err := d.db.Query(query, number, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query SMS: %w", err)
	}
	defer rows.Close()

	var messages []ReceivedSMS

	for rows.Next() {
		var msg ReceivedSMS
		var timestampStr, createdAtStr string

		err := rows.Scan(&msg.ID, &msg.Number, &msg.Content, &timestampStr, &createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		msg.Timestamp = parseTimestamp(timestampStr)
		msg.CreatedAt = parseTimestamp(createdAtStr)

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return messages, nil
}

// FindReceivedSMS searches for the most recent received SMS containing the given string (case-insensitive)
func (d *Database) FindReceivedSMS(search string) (*ReceivedSMS, error) {
	query := `
		SELECT id, number, content, timestamp, created_at
		FROM received_sms
		WHERE content LIKE '%' || ? || '%'
		ORDER BY timestamp DESC
		LIMIT 1
	`

	var msg ReceivedSMS
	var timestampStr, createdAtStr string

	err := d.db.QueryRow(query, search).Scan(&msg.ID, &msg.Number, &msg.Content, &timestampStr, &createdAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to search SMS: %w", err)
	}

	msg.Timestamp = parseTimestamp(timestampStr)
	msg.CreatedAt = parseTimestamp(createdAtStr)

	return &msg, nil
}

// CountReceivedSMS returns the total count of received SMS
func (d *Database) CountReceivedSMS() (int, error) {
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM received_sms").Scan(&count)
	return count, err
}

// SaveSentSMS stores a sent SMS in the database
func (d *Database) SaveSentSMS(number, content, status, errorMsg string) error {
	query := `INSERT INTO sent_sms (number, content, status, error) VALUES (?, ?, ?, ?)`

	_, err := d.db.Exec(query, number, content, status, errorMsg)
	if err != nil {
		return fmt.Errorf("failed to save sent SMS: %w", err)
	}

	return nil
}

// GetSentSMS retrieves all sent SMS messages with pagination
func (d *Database) GetSentSMS(limit, offset int) ([]SentSMS, error) {
	query := `
		SELECT id, number, content, status, COALESCE(error, ''), created_at
		FROM sent_sms
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := d.db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query sent SMS: %w", err)
	}
	defer rows.Close()

	var messages []SentSMS

	for rows.Next() {
		var msg SentSMS
		var createdAtStr string

		err := rows.Scan(&msg.ID, &msg.Number, &msg.Content, &msg.Status, &msg.Error, &createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		msg.CreatedAt = parseTimestamp(createdAtStr)

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return messages, nil
}

// GetSentSMSByNumber retrieves sent SMS messages to a specific number
func (d *Database) GetSentSMSByNumber(number string, limit, offset int) ([]SentSMS, error) {
	query := `
		SELECT id, number, content, status, COALESCE(error, ''), created_at
		FROM sent_sms
		WHERE number = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := d.db.Query(query, number, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query sent SMS: %w", err)
	}
	defer rows.Close()

	var messages []SentSMS

	for rows.Next() {
		var msg SentSMS
		var createdAtStr string

		err := rows.Scan(&msg.ID, &msg.Number, &msg.Content, &msg.Status, &msg.Error, &createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		msg.CreatedAt = parseTimestamp(createdAtStr)

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return messages, nil
}

// CountSentSMS returns the total count of sent SMS
func (d *Database) CountSentSMS() (int, error) {
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM sent_sms").Scan(&count)
	return count, err
}

// CountSentSMSByStatus returns the count of sent SMS by status
func (d *Database) CountSentSMSByStatus(status string) (int, error) {
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM sent_sms WHERE status = ?", status).Scan(&count)
	return count, err
}

// parseTimestamp tries multiple formats to parse a SQLite timestamp string
func parseTimestamp(s string) time.Time {
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

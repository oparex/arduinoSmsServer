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

		// Parse timestamps
		msg.Timestamp, _ = time.Parse("2006-01-02 15:04:05", timestampStr)
		msg.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)

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

		// Parse timestamps
		msg.Timestamp, _ = time.Parse("2006-01-02 15:04:05", timestampStr)
		msg.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return messages, nil
}

// CountReceivedSMS returns the total count of received SMS
func (d *Database) CountReceivedSMS() (int, error) {
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM received_sms").Scan(&count)
	return count, err
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

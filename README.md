# Arduino SMS Server

A complete Go API backend using the Gin framework for sending and receiving SMS messages through an Arduino MKR GSM 1400 connected via USB.

## Features

- **Send SMS** via RESTful API endpoint
- **Receive SMS** automatically from Arduino and store in SQLite database
- **Real serial communication** with Arduino over USB
- **Auto-discovery** of Arduino devices on available serial ports
- **Bidirectional communication** using JSON protocol
- **SQLite database** for persistent storage of received messages
- **Mock mode** for development/testing without hardware
- **Graceful shutdown** handling
- **Pagination** support for retrieving messages
- **Health check** and statistics endpoints

## Prerequisites

- Go 1.21 or higher
- Arduino MKR GSM 1400 with SIM card (for production use)
- GCC compiler (for CGO/SQLite support)
  - Linux: `apt-get install build-essential`
  - macOS: Install Xcode Command Line Tools
  - Windows: Install MinGW or TDM-GCC

## Installation

### Arduino Setup

1. Upload the Arduino sketch:
   - Open `arduino/sms_gateway/sms_gateway.ino` in Arduino IDE
   - Install the MKRGSM library (Tools > Manage Libraries)
   - Configure your SIM PIN in the sketch if needed
   - Upload to your Arduino MKR GSM 1400
   - See [arduino/README.md](arduino/README.md) for detailed instructions

2. Connect the Arduino to your computer via USB

### Go Backend Setup

1. Clone the repository:
```bash
git clone https://github.com/oparex/arduinoSmsServer.git
cd arduinoSmsServer
```

2. Install dependencies:
```bash
go mod download
```

3. Run the server (auto-discovery mode):
```bash
go run .
```

Or specify the serial port:
```bash
DEVICE_MODE=/dev/ttyACM0 go run .
```

Or run in mock mode (no Arduino required):
```bash
DEVICE_MODE=mock go run .
```

The server will start on `http://localhost:8080`

### Building

```bash
go build -o arduinoSmsServer
./arduinoSmsServer
```

## API Endpoints

### Health Check
```
GET /health
```

Response:
```json
{
  "status": "healthy",
  "service": "Arduino SMS Server",
  "connected": true,
  "mode": "auto"
}
```

### Send SMS
```
POST /send
```

Request body:
```json
{
  "number": "+1234567890",
  "content": "Your message here"
}
```

Response (success):
```json
{
  "status": "success",
  "message": "SMS sent to +1234567890"
}
```

Response (error):
```json
{
  "status": "error",
  "message": "Error description"
}
```

### Get Received SMS
```
GET /received?limit=50&offset=0
```

Query parameters:
- `limit` (optional): Number of messages to return (default: 50, max: 100)
- `offset` (optional): Number of messages to skip (default: 0)

Response:
```json
{
  "status": "success",
  "total": 150,
  "count": 50,
  "messages": [
    {
      "id": 1,
      "number": "+1234567890",
      "content": "Hello from sender",
      "timestamp": "2024-01-17T10:30:00Z",
      "created_at": "2024-01-17T10:30:05Z"
    }
  ]
}
```

### Get Received SMS by Number
```
GET /received/:number?limit=50&offset=0
```

Returns all SMS messages received from a specific phone number.

### Get Statistics
```
GET /stats
```

Response:
```json
{
  "status": "success",
  "total_received": 150,
  "connected": true,
  "mode": "auto"
}
```

## Usage Examples

### Send an SMS

Using curl:
```bash
curl -X POST http://localhost:8080/send \
  -H "Content-Type: application/json" \
  -d '{
    "number": "+1234567890",
    "content": "Hello from Arduino SMS Server!"
  }'
```

Using JavaScript (fetch):
```javascript
fetch('http://localhost:8080/send', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({
    number: '+1234567890',
    content: 'Hello from Arduino SMS Server!'
  })
})
.then(response => response.json())
.then(data => console.log(data));
```

### Retrieve Received SMS

```bash
# Get the latest 50 messages
curl http://localhost:8080/received

# Get messages with pagination
curl http://localhost:8080/received?limit=20&offset=40

# Get messages from a specific number
curl http://localhost:8080/received/+1234567890
```

### Check Health and Connection Status

```bash
curl http://localhost:8080/health
```

### Get Statistics

```bash
curl http://localhost:8080/stats
```

## Architecture

### Project Structure

```
arduinoSmsServer/
├── arduino/
│   ├── sms_gateway/
│   │   └── sms_gateway.ino    # Arduino firmware
│   └── README.md              # Arduino setup guide
├── main.go                    # Main application
├── serial.go                  # Serial communication handler
├── database.go                # SQLite database operations
├── go.mod                     # Go dependencies
└── README.md                  # This file
```

### Communication Protocol

The Arduino and Go backend communicate over USB serial (115200 baud) using JSON messages:

**Go → Arduino (Commands):**
```json
{"cmd":"send","number":"+1234567890","content":"message"}
{"cmd":"ping"}
```

**Arduino → Go (Responses/Events):**
```json
{"status":"ok","message":"SMS sent"}
{"status":"error","message":"error details"}
{"status":"ready","message":"SMS Gateway ready"}
{"event":"received","number":"+1234567890","content":"message","timestamp":"12:34:56"}
```

## Environment Variables

- `DEVICE_MODE`: Connection mode (default: `auto`)
  - `auto`: Auto-discover Arduino device
  - `mock`: Use mock serial connection (no hardware)
  - `/dev/ttyACM0` (or other path): Use specific serial port
- `PORT`: HTTP server port (default: `8080`)

## Database

The application uses SQLite to store received SMS messages. The database file `sms.db` is created automatically in the working directory.

### Schema

```sql
CREATE TABLE received_sms (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    number TEXT NOT NULL,
    content TEXT NOT NULL,
    timestamp DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

## Future Improvements

- Add authentication/API key support
- Implement rate limiting
- Add SMS queue management for failed sends
- Support for multiple Arduino devices
- Message delivery status tracking and confirmations
- WebSocket support for real-time SMS notifications
- SMS templates and scheduled sending
- Webhook support for received messages

## License

See LICENSE file for details.

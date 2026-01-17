# Arduino SMS Server

A simple Go API backend using the Gin framework for sending SMS messages through an Arduino MKR GSM 1400 connected via USB.

## Features

- RESTful API endpoint for sending SMS
- Mock serial connection (for development/testing)
- JSON request/response format
- Input validation
- Health check endpoint

## Prerequisites

- Go 1.21 or higher
- Arduino MKR GSM 1400 (for production use)

## Installation

1. Clone the repository:
```bash
git clone https://github.com/oparex/arduinoSmsServer.git
cd arduinoSmsServer
```

2. Install dependencies:
```bash
go mod download
```

3. Run the server:
```bash
go run main.go
```

The server will start on `http://localhost:8080`

## API Endpoints

### Health Check
```
GET /health
```

Response:
```json
{
  "status": "healthy",
  "service": "Arduino SMS Server"
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

## Usage Example

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

## Current Status

The serial connection is currently mocked for development purposes. The application logs simulated SMS sending operations to the console.

## Future Improvements

- Replace mock serial connection with actual Arduino serial communication
- Add authentication/API key support
- Implement rate limiting
- Add SMS queue management
- Support for multiple Arduino devices
- Message delivery status tracking

## License

See LICENSE file for details.

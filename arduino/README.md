# Arduino SMS Gateway

This folder contains the Arduino sketch for the MKR GSM 1400 board that acts as an SMS gateway.

## Hardware Requirements

- Arduino MKR GSM 1400
- SIM card with SMS capability
- USB cable for connection to host computer

## Libraries Required

Install these libraries through the Arduino IDE Library Manager:

- MKRGSM (by Arduino)

## Installation

1. Open `sms_gateway/sms_gateway.ino` in Arduino IDE
2. Connect your Arduino MKR GSM 1400 via USB
3. Select the correct board: Tools > Board > Arduino MKR GSM 1400
4. Select the correct port: Tools > Port
5. Upload the sketch

## PIN Configuration

If your SIM card requires a PIN, edit this line in the sketch:

```cpp
#define PIN_NUMBER "1234"  // Replace with your PIN
```

If no PIN is required, leave it empty:

```cpp
#define PIN_NUMBER ""
```

## Serial Protocol

The Arduino communicates with the Go backend using JSON messages over USB serial (115200 baud).

### Commands (Go -> Arduino)

**Send SMS:**
```json
{"cmd":"send","number":"+1234567890","content":"Your message here"}
```

**Ping:**
```json
{"cmd":"ping"}
```

### Responses (Arduino -> Go)

**Success:**
```json
{"status":"ok","message":"SMS sent to +1234567890"}
```

**Error:**
```json
{"status":"error","message":"Error description"}
```

**Status Info:**
```json
{"status":"info","message":"Connecting to GSM network..."}
```

**Ready:**
```json
{"status":"ready","message":"SMS Gateway ready"}
```

### Events (Arduino -> Go)

**Received SMS:**
```json
{"event":"received","number":"+1234567890","content":"Message content","timestamp":"12:34:56"}
```

## LED Indicators

The Arduino MKR GSM 1400 has built-in LEDs:
- Power LED: Board is powered
- Pin 13 LED: Can be used for custom status indication

## Troubleshooting

### No GSM Connection
- Check SIM card is properly inserted
- Verify SIM card has network coverage
- Check PIN number if required
- Wait for network registration (can take 30-60 seconds)

### Serial Communication Issues
- Verify correct baud rate (115200)
- Check USB cable connection
- Ensure correct port is selected
- Try resetting the Arduino

### SMS Not Sending
- Check GSM connection status
- Verify phone number format (use international format with +)
- Check SIM card credit/plan supports SMS
- Message content should not be too long (160 chars standard)

## Power Consumption

The MKR GSM 1400 can consume significant power during GSM transmission. For battery-powered applications, consider:
- Using deep sleep between operations
- Reducing SMS check frequency
- Optimizing GSM power settings

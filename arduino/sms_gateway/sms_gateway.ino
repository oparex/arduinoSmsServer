/*
  Arduino MKR GSM 1400 - SMS Gateway

  This sketch implements a USB serial gateway for sending and receiving SMS messages.
  It communicates with the Go backend over USB serial connection.

  Protocol:
  - Commands are sent as JSON objects terminated with newline
  - Send SMS: {"cmd":"send","number":"+1234567890","content":"message"}
  - Response: {"status":"ok","message":"SMS sent"} or {"status":"error","message":"error details"}
  - Incoming SMS: {"event":"received","number":"+1234567890","content":"message","timestamp":"YYYY-MM-DD HH:MM:SS"}

  Power management:
  - GSM connects on boot, then auto-disconnects after 60 seconds of inactivity
  - "wakeup" command reconnects GSM; "send" auto-connects if disconnected
  - Every response includes "gsm" field ("connected" or "disconnected")
*/

#include <MKRGSM.h>

// PIN Number if required (leave empty if not needed)
#define PIN_NUMBER ""

// Initialize the library instances
GSM gsmAccess;
GSM_SMS sms;

// Buffer for incoming serial data
String serialBuffer = "";
const int MAX_BUFFER_SIZE = 512;

// Last check time for incoming SMS
unsigned long lastSMSCheck = 0;
const unsigned long SMS_CHECK_INTERVAL = 5000; // Check every 5 seconds

// Connection state
bool gsmConnected = false;

// Inactivity timer
unsigned long lastActivityTime = 0;
const unsigned long INACTIVITY_TIMEOUT = 60000; // 60 seconds

void setup() {
  // Initialize serial communications
  Serial.begin(115200);
  while (!Serial) {
    ; // Wait for serial port to connect
  }

  sendInfo("Arduino SMS Gateway starting...");

  // Initialize GSM connection
  connectGSM();

  sendReady("SMS Gateway ready");
}

void loop() {
  // Always read serial commands (even when GSM disconnected, so wakeup works)
  while (Serial.available() > 0) {
    char c = Serial.read();

    if (c == '\n') {
      // Process complete command
      processCommand(serialBuffer);
      serialBuffer = "";
    } else if (serialBuffer.length() < MAX_BUFFER_SIZE) {
      serialBuffer += c;
    } else {
      // Buffer overflow, clear it
      serialBuffer = "";
      sendError("Buffer overflow");
    }
  }

  // Only check incoming SMS when GSM connected
  if (gsmConnected) {
    if (millis() - lastSMSCheck > SMS_CHECK_INTERVAL) {
      checkIncomingSMS();
      lastSMSCheck = millis();
    }

    // Check inactivity timeout
    if (millis() - lastActivityTime > INACTIVITY_TIMEOUT) {
      disconnectGSM();
    }
  }

  delay(100);
}

bool connectGSM() {
  sendInfo("Connecting to GSM network...");

  // Start GSM connection
  if (gsmAccess.begin(PIN_NUMBER) == GSM_READY) {
    gsmConnected = true;
    resetActivityTimer();
    sendGSMState();
    sendInfo("Connected to GSM network");
    return true;
  } else {
    gsmConnected = false;
    sendGSMState();
    sendError("Failed to connect to GSM network");
    return false;
  }
}

void disconnectGSM() {
  gsmAccess.shutdown();
  gsmConnected = false;
  sendGSMState();
  sendInfo("GSM disconnected due to inactivity");
}

void resetActivityTimer() {
  lastActivityTime = millis();
}

void processCommand(String command) {
  // Simple JSON parsing (basic implementation)
  command.trim();

  if (command.length() == 0) {
    return;
  }

  // Extract command type
  int cmdStart = command.indexOf("\"cmd\"");
  if (cmdStart == -1) {
    sendError("Invalid command format");
    return;
  }

  // Check command type
  if (command.indexOf("\"send\"") != -1) {
    handleSendSMS(command);
  } else if (command.indexOf("\"ping\"") != -1) {
    resetActivityTimer();
    sendResponse("ok", "pong");
  } else if (command.indexOf("\"wakeup\"") != -1) {
    if (!gsmConnected) {
      connectGSM();
    } else {
      resetActivityTimer();
    }
    sendResponse("ok", "wakeup acknowledged");
  } else if (command.indexOf("\"status\"") != -1) {
    sendResponse("ok", gsmConnected ? "gsm connected" : "gsm disconnected");
  } else {
    sendError("Unknown command");
  }
}

void handleSendSMS(String command) {
  // Extract number
  String number = extractJSONValue(command, "number");
  if (number.length() == 0) {
    sendError("Missing phone number");
    return;
  }

  // Extract content
  String content = extractJSONValue(command, "content");
  if (content.length() == 0) {
    sendError("Missing message content");
    return;
  }

  // Auto-connect GSM if disconnected
  if (!gsmConnected) {
    if (!connectGSM()) {
      sendError("Failed to connect GSM for sending");
      return;
    }
  }

  resetActivityTimer();

  // Send SMS
  sms.beginSMS(number.c_str());
  sms.print(content);

  if (sms.endSMS()) {
    sendResponse("ok", "SMS sent to " + number);
  } else {
    sendError("Failed to send SMS");
  }
}

void checkIncomingSMS() {
  if (!gsmConnected) {
    return;
  }

  // Check if there are any SMS available
  if (sms.available()) {
    resetActivityTimer();

    String number = "";
    String message = "";

    // Get sender number
    char senderNumber[20];
    sms.remoteNumber(senderNumber, 20);
    number = String(senderNumber);

    // Read message content
    int c;
    while ((c = sms.read()) != -1) {
      message += (char)c;
    }

    // Delete the message from SIM
    sms.flush();

    // Send received SMS to Go app
    sendReceivedSMS(number, message);
  }
}

void sendReceivedSMS(String number, String content) {
  // Get timestamp (basic format)
  String timestamp = getTimestamp();

  Serial.print("{\"event\":\"received\",\"number\":\"");
  Serial.print(escapeJSON(number));
  Serial.print("\",\"content\":\"");
  Serial.print(escapeJSON(content));
  Serial.print("\",\"timestamp\":\"");
  Serial.print(timestamp);
  Serial.print("\",\"gsm\":\"");
  Serial.print(gsmConnected ? "connected" : "disconnected");
  Serial.println("\"}");
}

void sendGSMState() {
  Serial.print("{\"event\":\"gsm_state\",\"gsm\":\"");
  Serial.print(gsmConnected ? "connected" : "disconnected");
  Serial.println("\"}");
}

void sendResponse(String status, String message) {
  Serial.print("{\"status\":\"");
  Serial.print(status);
  Serial.print("\",\"message\":\"");
  Serial.print(escapeJSON(message));
  Serial.print("\",\"gsm\":\"");
  Serial.print(gsmConnected ? "connected" : "disconnected");
  Serial.println("\"}");
}

void sendError(String message) {
  sendResponse("error", message);
}

void sendInfo(String message) {
  Serial.print("{\"status\":\"info\",\"message\":\"");
  Serial.print(escapeJSON(message));
  Serial.print("\",\"gsm\":\"");
  Serial.print(gsmConnected ? "connected" : "disconnected");
  Serial.println("\"}");
}

void sendReady(String message) {
  Serial.print("{\"status\":\"ready\",\"message\":\"");
  Serial.print(escapeJSON(message));
  Serial.print("\",\"gsm\":\"");
  Serial.print(gsmConnected ? "connected" : "disconnected");
  Serial.println("\"}");
}

String extractJSONValue(String json, String key) {
  String searchKey = "\"" + key + "\":\"";
  int startIndex = json.indexOf(searchKey);

  if (startIndex == -1) {
    return "";
  }

  startIndex += searchKey.length();
  int endIndex = json.indexOf("\"", startIndex);

  if (endIndex == -1) {
    return "";
  }

  return json.substring(startIndex, endIndex);
}

String escapeJSON(String str) {
  String result = "";
  for (unsigned int i = 0; i < str.length(); i++) {
    char c = str[i];
    if (c == '"' || c == '\\') {
      result += '\\';
    }
    result += c;
  }
  return result;
}

String getTimestamp() {
  // For now, return a simple uptime-based timestamp
  // In production, you could use GSM time or an RTC module
  unsigned long seconds = millis() / 1000;
  unsigned long minutes = seconds / 60;
  unsigned long hours = minutes / 60;

  String timestamp = String(hours % 24) + ":";
  timestamp += String(minutes % 60) + ":";
  timestamp += String(seconds % 60);

  return timestamp;
}

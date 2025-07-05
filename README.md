# MQTT to ESC/POS Printer Bridge

A simple Go bridge that connects MQTT messages to ESC/POS thermal printers. Perfect for Home Assistant and automation scenarios like printing shopping lists, receipts, and labels.

## Features

- **Text Printing**: Print formatted text with size, bold, underline, and justification
- **Image Printing**: Print PNG, JPEG, and GIF images with automatic scaling
- **QR Codes**: Print QR codes with configurable error correction
- **Barcodes**: Print EAN13, EAN8, and UPC-A barcodes
- **Paper Control**: Separate feed and cut commands for flexible workflows
- **Home Assistant Integration**: Seamless MQTT integration
- **Auto-reconnection**: Automatic MQTT reconnection on connection loss

## Quick Start

1. **Build the Application**
```bash
git clone <repository-url>
cd mqtt2escpos
go build -o mqtt2escpos
```

2. **Configure Environment Variables**
```bash
export MQTT_BROKER="your-mqtt-broker:1883"
export MQTT_USERNAME="your-mqtt-username"
export MQTT_PASSWORD="your-mqtt-password"
export MQTT_TOPIC="printer/commands"
export PRINTER_HOST="192.168.1.100"
export PRINTER_PORT="9100"
```

3. **Run the Bridge**
```bash
./mqtt2escpos
```

## Home Assistant Examples

### Text Printing
```yaml
service: mqtt.publish
data:
  topic: printer/commands
  payload: >
    {
      "printer_name": "HomeAssistant",
      "message": "Hello from Home Assistant!"
    }
  qos: 0
  retain: false
  evaluate_payload: true
```

### Formatted Text
```yaml
service: mqtt.publish
data:
  topic: printer/commands
  payload: >
    {
      "printer_name": "HomeAssistant",
      "title": "Shopping List",
      "message": "Items to buy:\n- Milk\n- Bread\n- Eggs",
      "size": 2,
      "bold": true,
      "justify": "center"
    }
  qos: 0
  retain: false
  evaluate_payload: true
```

### Image Printing
```yaml
service: mqtt.publish
data:
  topic: printer/commands
  payload: >
    {
      "printer_name": "HomeAssistant",
      "image": "<base64-image-data>",
      "image_format": "png"
    }
  qos: 0
  retain: false
  evaluate_payload: true
```

### QR Code
```yaml
service: mqtt.publish
data:
  topic: printer/commands
  payload: >
    {
      "printer_name": "HomeAssistant",
      "qr_code": "https://home-assistant.io"
    }
  qos: 0
  retain: false
  evaluate_payload: true
```

### Barcode
```yaml
service: mqtt.publish
data:
  topic: printer/commands
  payload: >
    {
      "printer_name": "HomeAssistant",
      "barcode": "1234567890123"
    }
  qos: 0
  retain: false
  evaluate_payload: true
```

### Paper Control Commands

**Feed paper:**
```yaml
service: mqtt.publish
data:
  topic: printer/commands
  payload: >
    {
      "printer_name": "HomeAssistant",
      "command": "feed",
      "lines": 3
    }
  qos: 0
  retain: false
  evaluate_payload: true
```

**Cut paper:**
```yaml
service: mqtt.publish
data:
  topic: printer/commands
  payload: >
    {
      "printer_name": "HomeAssistant",
      "command": "cut"
    }
  qos: 0
  retain: false
  evaluate_payload: true
```

**Feed and cut:**
```yaml
service: mqtt.publish
data:
  topic: printer/commands
  payload: >
    {
      "printer_name": "HomeAssistant",
      "command": "feed_and_cut",
      "lines": 2
    }
  qos: 0
  retain: false
  evaluate_payload: true
```

## Full Home Assistant Automation Example

This automation prints a logo, a message, a footer, and then cuts the paper:

```yaml
alias: Print Shopping List with Logo and Footer
sequence:
  # Print the logo image
  - service: mqtt.publish
    data:
      topic: printer/commands
      payload: >
        {
          "printer_name": "HomeAssistant",
          "image": "{{ states('sensor.logo_base64') }}",
          "image_format": "png"
        }
      qos: 0
      retain: false
      evaluate_payload: true

  # Feed paper after image
  - service: mqtt.publish
    data:
      topic: printer/commands
      payload: >
        {
          "printer_name": "HomeAssistant",
          "command": "feed",
          "lines": 1
        }
      qos: 0
      retain: false
      evaluate_payload: true

  # Print the main message (e.g., shopping list)
  - service: mqtt.publish
    data:
      topic: printer/commands
      payload: >
        {
          "printer_name": "HomeAssistant",
          "title": "Shopping List",
          "message": "- Milk\n- Bread\n- Eggs",
          "size": 2,
          "bold": true,
          "justify": "left"
        }
      qos: 0
      retain: false
      evaluate_payload: true

  # Print the footer
  - service: mqtt.publish
    data:
      topic: printer/commands
      payload: >
        {
          "printer_name": "HomeAssistant",
          "message": "\n\nHappy Shopping!",
          "size": 1,
          "bold": false,
          "justify": "center"
        }
      qos: 0
      retain: false
      evaluate_payload: true

  # Cut the paper
  - service: mqtt.publish
    data:
      topic: printer/commands
      payload: >
        {
          "printer_name": "HomeAssistant",
          "command": "cut"
        }
      qos: 0
      retain: false
      evaluate_payload: true
mode: single
```

## Message Format

| Field         | Type   | Description                                 |
|-------------- |--------|---------------------------------------------|
| printer_name  | string | Printer identifier                          |
| message       | string | Text to print                               |
| title         | string | Optional title (large, bold, centered)      |
| image         | string | Base64 encoded image data                   |
| image_format  | string | Image format: png, jpeg, gif                |
| qr_code       | string | QR code content                             |
| barcode       | string | Barcode content (EAN13, EAN8, UPC-A)        |
| size          | int    | Text size (1-8)                             |
| bold          | bool   | Bold text                                   |
| underline     | bool   | Underlined text                             |
| justify       | string | left, center, right                         |
| command       | string | feed, cut, feed_and_cut                     |
| lines         | int    | Number of lines to feed (for feed commands) |

## Configuration

| Variable        | Default           | Description                  |
|-----------------|-------------------|------------------------------|
| MQTT_BROKER     | localhost:1883    | MQTT broker address:port     |
| MQTT_USERNAME   | (empty)           | MQTT username                |
| MQTT_PASSWORD   | (empty)           | MQTT password                |
| MQTT_TOPIC      | printer/commands  | MQTT topic to listen on      |
| PRINTER_HOST    | 192.168.7.25      | Printer IP address           |
| PRINTER_PORT    | 9100              | Printer port                 |

---

*Requires Go 1.24+, an MQTT broker, and an ESC/POS compatible thermal printer on your network.*

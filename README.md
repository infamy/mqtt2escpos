# MQTT to ESC/POS Printer Bridge

A Go-based bridge that connects MQTT messages to ESC/POS thermal printers, designed specifically for Home Assistant integration. This application listens for MQTT messages and prints them using ESC/POS commands, making it perfect for smart home automation scenarios like printing shopping lists, receipts, and custom labels.

## Features

- **Home Assistant Integration**: Seamless integration with Home Assistant MQTT services
- **ESC/POS Support**: Full support for ESC/POS thermal printers
- **Image Printing**: Print PNG, JPEG, and GIF images with automatic scaling
- **Rich Formatting**: Support for bold, underline, text sizing, and justification
- **QR Code Generation**: Print QR codes with configurable error correction
- **Barcode Support**: Print EAN13, EAN8, and UPC-A barcodes
- **Auto-reconnection**: Automatic MQTT reconnection on connection loss
- **Graceful Shutdown**: Proper signal handling for clean exits

## Quick Start

### 1. Build the Application

```bash
git clone <repository-url>
cd mqtt2escpos
go build -o mqtt2escpos
```

### 2. Configure Environment Variables

Create a configuration file or set environment variables:

```bash
# MQTT Configuration
export MQTT_BROKER="your-home-assistant-ip:1883"
export MQTT_USERNAME="your-mqtt-username"
export MQTT_PASSWORD="your-mqtt-password"
export MQTT_TOPIC="printer/commands"

# Printer Configuration
export PRINTER_HOST="192.168.1.100"
export PRINTER_PORT="9100"
```

### 3. Run the Bridge

```bash
./mqtt2escpos
```

## Home Assistant Integration

### Basic Usage

In Home Assistant, go to **Developer Tools > Services** and use the `mqtt.publish` service:

**Service**: `mqtt.publish`  
**Topic**: `printer/commands`  
**Payload**:
```json
{
  "printer_name": "KitchenPrinter",
  "message": "Hello from Home Assistant!"
}
```

### Advanced Formatting

```json
{
  "printer_name": "KitchenPrinter",
  "title": "Shopping List",
  "message": "Items to buy:\n- Milk\n- Bread\n- Eggs",
  "size": 2,
  "bold": true,
  "justify": "center"
}
```

### Image Printing

Print images by converting them to base64 and including them in the payload:

```json
{
  "printer_name": "KitchenPrinter",
  "title": "Company Logo",
  "message": "Printing logo below",
  "image": "iVBORw0KGgoAAAANSUhEUgAA...",
  "image_format": "png"
}
```

**Supported Image Formats**: PNG, JPEG, GIF

**To convert an image to base64**:
```bash
base64 -i your-image.png | tr -d '\n'
```

### QR Codes and Barcodes

```json
{
  "printer_name": "KitchenPrinter",
  "qr_code": "https://home-assistant.io",
  "barcode": "1234567890123"
}
```

## Shopping List Automation

Here's a complete Home Assistant automation for printing shopping lists:

```yaml
alias: Print Shopping List
sequence:
  - variables:
      shopping_list_name: "Shopping List"
      todo_entity: "todo.shopping_list"

  - data:
      status: needs_action
    target:
      entity_id: "{{ todo_entity }}"
    response_variable: shopping_list_response
    action: todo.get_items

  - variables:
      shopping_list_message: >
        {%- set items = shopping_list_response[todo_entity]['items'] -%}
        {%- for item in items -%}
        [ ] {{ item['summary'] }}
        {%- if not loop.last -%}
        {{ "\n" }}
        {%- endif -%}
        {%- endfor -%}

  - service: mqtt.publish
    data:
      topic: printer/commands
      payload: >
        {
          "printer_name": "HomeAssistant",
          "title": "{{ shopping_list_name }}",
          "message": "{{ shopping_list_message | replace('\n', '\\n') }}\n\n\nWoodland Eatery",
          "size": 1,
          "bold": false,
          "justify": "left"
        }
      evaluate_payload: true
      qos: 0
      retain: false
mode: single
```

## Message Format

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `printer_name` | string | Printer identifier (for logging) | `"KitchenPrinter"` |
| `title` | string | Document title (printed in large, bold, centered text) | `"Shopping List"` |
| `message` | string | Text to print | `"Hello, World!"` |
| `size` | int | Text size (1-8) | `2` |
| `bold` | bool | Bold text | `true` |
| `underline` | bool | Underlined text | `false` |
| `justify` | string | Text alignment (`left`, `center`, `right`) | `"center"` |
| `qr_code` | string | QR code content | `"https://example.com"` |
| `barcode` | string | Barcode content (EAN13, EAN8, UPC-A) | `"1234567890123"` |
| `image` | string | Base64 encoded image data | `"iVBORw0KGgoAAAANSUhEUgAA..."` |
| `image_format` | string | Image format (`png`, `jpeg`, `gif`) | `"png"` |

## Testing

### Test Image Printing

Use the provided test script to verify image printing functionality:

```bash
./tests/test-image-simple.sh
```

This script creates a test image and provides the MQTT payload to send it to your printer.

### Test Basic Functionality

```bash
./tests/test_printer.sh quick
```

### All Test Scripts

See the `tests/` folder for additional test scripts:
- `test_mqtt.sh` - Test MQTT connectivity
- `test_printer.sh` - Comprehensive printer testing
- `test-image.sh` - Image printing with automatic MQTT sending
- `test-local.sh` - Local development testing

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MQTT_BROKER` | `localhost:1883` | MQTT broker address and port |
| `MQTT_USERNAME` | (empty) | MQTT username |
| `MQTT_PASSWORD` | (empty) | MQTT password |
| `MQTT_TOPIC` | `printer/commands` | MQTT topic to listen for print commands |
| `PRINTER_HOST` | `192.168.7.25` | Printer IP address |
| `PRINTER_PORT` | `9100` | Printer port (usually 9100 for ESC/POS) |

## Use Cases

- **Shopping Lists**: Print shopping lists from Home Assistant todo entities
- **Event Reminders**: Print daily schedules or event reminders
- **Receipt Printing**: Generate and print custom receipts
- **Custom Labels**: Print formatted labels with QR codes or barcodes
- **Logo Printing**: Print company logos or custom images
- **IoT Notifications**: Print alerts and notifications from IoT devices

## Prerequisites

- Go 1.24 or later
- MQTT broker (Home Assistant MQTT add-on recommended)
- ESC/POS compatible thermal printer
- Network connectivity between the application and printer

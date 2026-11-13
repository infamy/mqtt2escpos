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

## Home Assistant Add-on

This project can also be used as a Home Assistant add-on. The add-on files are included in the root of this repository.

### Installation as Local Add-on

1. **Copy the Repository**: Copy this entire folder to your Home Assistant `addons` directory:
   ```bash
   # If using Samba or SSH access to your Home Assistant instance
   cp -r /path/to/mqtt2escpos /path/to/homeassistant/addons/
   ```
   
   The typical add-ons directory locations are:
   - **Home Assistant OS**: `/addons/` (accessible via Samba or SSH)
   - **Home Assistant Supervised**: `/usr/share/hassio/addons/local/`
   - **Docker**: Mounted volume containing add-ons

2. **Refresh Add-ons**: In Home Assistant:
   - Go to **Settings > Add-ons > Add-on Store**
   - Click the **Update** button (🔄) in the top right corner
   - This will refresh the add-on list and show your local add-ons

3. **Install the Add-on**:
   - Look for "MQTT to ESC/POS Printer Bridge" in the **Local Add-ons** section
   - Click on it and then click **Install**
   - Wait for the installation to complete

4. **Configure and Start**:
   - Click **Configuration** to set up your MQTT broker and printer settings
   - Click **Start** to run the add-on
   - Check the **Logs** tab to verify it's running correctly

### Configuration Options

The add-on can be configured through the Home Assistant UI with these options:

| Option | Default | Description |
|--------|---------|-------------|
| `mqtt_broker` | `core-mosquitto` | MQTT broker address (use `core-mosquitto` for Home Assistant's built-in broker) |
| `mqtt_username` | (empty) | MQTT username (leave empty if no authentication) |
| `mqtt_password` | (empty) | MQTT password (leave empty if no authentication) |
| `mqtt_topic` | `printer/commands` | MQTT topic to listen for print commands |
| `printer_host` | `192.168.7.25` | Your printer's IP address |
| `printer_port` | `9100` | Printer port (usually 9100 for network printers) |
| `printer_profile` | `TM-T88VI` | Printer model profile |
| `image_timeout` | `30` | How long a job containing an image may take to send to the printer (seconds) |
| `max_image_size` | `1048576` | Maximum size of the base64 `image` field in bytes; larger jobs are rejected |
| `ha_discovery` | `true` | Create a "Receipt Printer" device in Home Assistant via MQTT discovery |
| `status_interval` | `60` | How often to check the printer's status (seconds, minimum 10) |

### Home Assistant device

With `ha_discovery` on, the bridge creates a **Receipt Printer** device with:

| Entity | What it shows |
|--------|---------------|
| Bridge | Whether the bridge is connected to MQTT |
| Printer | Whether the printer is reachable on the network |
| Paper low / Paper out / Cover open | Read from the printer with ESC/POS status queries (unknown if the printer doesn't answer them) |
| Feed / Cut / Feed and cut | Buttons that send the matching paper command |

The raw status is published (retained) to `mqtt2escpos/status` (`online`/`offline`) and `mqtt2escpos/printer` (JSON).

### Not losing jobs

The bridge keeps a persistent MQTT session, so the broker holds jobs sent while the bridge is restarting
and delivers them when it reconnects. This only works for messages published with **QoS 1**, so add
`qos: 1` to your `mqtt.publish` actions:

```yaml
action: mqtt.publish
data:
  topic: printer/commands
  qos: 1
  payload: '{"message": "Hello"}'
```

### How it works

- The add-on builds and runs the Go-based MQTT to ESC/POS bridge inside a Home Assistant add-on container
- All configuration is passed via environment variables from the add-on options
- The add-on uses the same MQTT and printer settings as the standalone app
- It automatically connects to your MQTT broker and listens for print commands
- All the Home Assistant examples above work exactly the same with the add-on

### Troubleshooting

- **Add-on not appearing**: Make sure you copied the entire folder and clicked the Update button
- **Connection issues**: Check that your printer IP and port are correct
- **MQTT issues**: Verify your MQTT broker settings and credentials
- **Logs**: Always check the add-on logs for detailed error messages

For advanced usage and support, see the rest of this README.

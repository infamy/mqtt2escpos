# MQTT to ESC/POS Printer Bridge Add-on for Home Assistant

This directory contains the Home Assistant add-on for the MQTT to ESC/POS Printer Bridge.

For full project details, usage, and advanced configuration, see the root [README.md](../README.md).

## Installation

1. Copy this `mqtt2escpos` directory to your Home Assistant `addons` folder (or add to a custom add-on repository).
2. In Home Assistant, go to **Settings > Add-ons > Add-on Store** and click the three dots (⋮) > **Repositories**. Add your repository if needed.
3. Find "MQTT to ESC/POS Printer Bridge" and install it.
4. Configure the add-on options as needed.
5. Start the add-on and check the logs for status.

## Configuration Options

See [config.yaml](./config.yaml) for all available options and defaults.

## How it works

- The add-on builds and runs the Go-based MQTT to ESC/POS bridge inside a Home Assistant add-on container.
- All configuration is passed via environment variables from the add-on options.
- The add-on uses the same MQTT and printer settings as the standalone app.

## Support

For issues, see the main project [README.md](../README.md) or open an issue on GitHub. 